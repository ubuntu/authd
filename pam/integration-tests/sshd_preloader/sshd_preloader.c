// TiCS: disabled // This is a test helper.

#define _GNU_SOURCE 1
#include <assert.h>
#include <stdlib.h>
#include <stdio.h>
#include <stdatomic.h>
#include <stdbool.h>
#include <unistd.h>
#include <sys/types.h>
#include <sys/stat.h>
#include <dlfcn.h>
#include <string.h>
#include <pwd.h>
#include <limits.h>

#define AUTHD_TEST_SHELL "/bin/sh"
#define AUTHD_TEST_GECOS ""
#define AUTHD_DEFAULT_SSH_PAM_SERVICE_NAME "sshd"
#define AUTHD_SPECIAL_USER_ACCEPT_ALL "authd-test-user-sshd-accept-all"

static struct passwd passwd_entities[512];

__attribute__((constructor))
void constructor (void)
{
  fprintf (stderr, "sshd_preloader [%d]: Library loaded\n", getpid ());
}

__attribute__((destructor))
void destructor (void)
{
  fprintf (stderr, "sshd_preloader [%d]: Library unloaded\n", getpid ());
}

static const char *
get_home_path (void)
{
  const char *home_path = getenv ("AUTHD_TEST_SSH_HOME");

  if (home_path == NULL)
    return "/not-existing-home";

  return home_path;
}

static bool
is_valid_test_user (const char *name)
{
  static const char *test_user = NULL;

  if (!test_user)
    test_user = getenv ("AUTHD_TEST_SSH_USER");

  if (test_user == NULL || *test_user == '\0')
    return false;

  if (strcmp (test_user, name) == 0)
    return true;

  if (strcmp (test_user, AUTHD_SPECIAL_USER_ACCEPT_ALL) != 0)
    return false;

  /* Here we accept all the users supported by the example broker */
  if (strncmp (name, "user", 4) == 0 && strlen (name) > 4)
    return true;

  /* Further special case for the 'r' user */
  if (strcmp (name, "r") == 0)
    return true;

  return false;
}

/*
 * This overrides allows us to manually handle the getpwnam() ensuring that
 * we reply a fake user only when an expected fake user is requested.
 * To handle this we could even have used __nss_configure_lookup()
 * with a fake module or our own, but this preloader is meant to be for
 * testing the behavior of the PAM module only and we want it to be fully
 * predictable for each test.
 */
struct passwd *
getpwnam (const char *name)
{
  static atomic_int last_entity_idx;
  struct passwd *passwd_entity;
  int entity_idx;

  if (!is_valid_test_user (name))
    {
      struct passwd * (*orig_getpwnam) (const char *name);

      orig_getpwnam = dlsym (RTLD_NEXT, "getpwnam");
      return orig_getpwnam (name);
    }

  fprintf (stderr, "sshd_preloader: Simulating to be user %s\n", name);

  entity_idx = atomic_fetch_add_explicit (&last_entity_idx, 1,
                                          memory_order_relaxed);
  assert (entity_idx < sizeof (passwd_entities) / sizeof (*passwd_entities));
  passwd_entity = &passwd_entities[entity_idx];
  passwd_entity->pw_shell = AUTHD_TEST_SHELL;
  passwd_entity->pw_gecos = AUTHD_TEST_GECOS;

  /* We're simulating to be the same user running the test but with another
   * name and HOME directory, so that we won't touch the user settings, but
   * it's still enough to trick sshd.
   */
  passwd_entity->pw_uid = getuid ();
  passwd_entity->pw_gid = getgid ();
  passwd_entity->pw_name = (char *) name;
  passwd_entity->pw_dir = (char *) get_home_path ();

  return passwd_entity;
}

FILE *
fopen (const char *pathname, const char *mode)
{
  static FILE * (*orig_fopen) (const char *pathname, const char *mode) = NULL;
  const char *service_path;

  if (!orig_fopen)
    orig_fopen = dlsym (RTLD_NEXT, "fopen");

  service_path = getenv ("AUTHD_TEST_SSH_PAM_SERVICE");

  if (service_path == NULL || pathname == NULL)
    return orig_fopen (pathname, mode);

  if (strcmp (pathname, "/etc/pam.d/" AUTHD_DEFAULT_SSH_PAM_SERVICE_NAME) == 0 ||
      strcmp (pathname, "/usr/lib/pam.d/" AUTHD_DEFAULT_SSH_PAM_SERVICE_NAME) == 0)
    {
      fprintf (stderr, "sshd_preloader [%d]: Trying to open '%s', "
               "but redirecting instead to '%s'\n",
               getpid (), pathname, service_path);
      pathname = service_path;
    }

  return orig_fopen (pathname, mode);
}
