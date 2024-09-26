#define _GNU_SOURCE 1
#include <stdlib.h>
#include <stdio.h>
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

__attribute__((constructor))
void constructor (void)
{
  fprintf (stderr, "sshd_preloader: Library loaded (parent pid: %d)!\n",
           getpid ());
}

__attribute__((destructor))
void destructor (void)
{
  fprintf (stderr, "sshd_preloader: Library unloaded (parent pid: %d)!\n",
           getpid ());
}

/*
 * Note: none of this code is meant to be thread-safe, but we don't need it for
 * the way we're using it in our tests.
 */

static const char *
get_home_path (void)
{
  const char *home_path = getenv ("AUTHD_TEST_SSH_HOME");

  if (home_path == NULL)
    return "/not-existing-home";

  return home_path;
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
  static const char *test_user = NULL;

  if (!test_user)
    test_user = getenv ("AUTHD_TEST_SSH_USER");

  if (test_user == NULL || strcmp (test_user, name) != 0)
    {
      struct passwd * (*orig_getpwnam) (const char *name);

      orig_getpwnam = dlsym (RTLD_NEXT, "getpwnam");
      return orig_getpwnam (name);
    }

  fprintf (stderr, "sshd_preloader: Simulating to be user %s\n", name);

  static struct passwd passwd_entity = {
    .pw_shell = AUTHD_TEST_SHELL,
    .pw_gecos = AUTHD_TEST_GECOS,
  };

  /* We're simulating to be the same user running the test but with another
   * name and HOME directory, so that we won't touch the user settings, but
   * it's still enough to trick sshd.
   */
  passwd_entity.pw_uid = getuid ();
  passwd_entity.pw_gid = getgid ();
  passwd_entity.pw_name = (char *) name;
  passwd_entity.pw_dir = (char *) get_home_path ();

  return &passwd_entity;
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
      fprintf (stderr, "sshd_preloader: Trying to open '%s', "
               "but redirecting instead to '%s'\n", pathname, service_path);
      pathname = service_path;
    }

  return orig_fopen (pathname, mode);
}

