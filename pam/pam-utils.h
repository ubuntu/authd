#include <stdlib.h>
#include <string.h>
#include <stdbool.h>
#include <unistd.h>
#include <gdm/gdm-pam-extensions.h>
#include <security/pam_modules.h>

char *argv_string_get (const char **argv,
                       unsigned     i);

char *get_user (pam_handle_t *pamh,
                const char   *prompt);

const char *get_module_name (pam_handle_t *pamh);

struct pam_response *send_msg (pam_handle_t *pamh,
                               const char   *msg,
                               int           style);

inline bool
gdm_choices_list_supported (void)
{
  return GDM_PAM_EXTENSION_SUPPORTED (GDM_PAM_EXTENSION_CHOICE_LIST);
}

GdmPamExtensionChoiceListRequest *gdm_choices_request_create (const char *,
                                                              size_t);

void gdm_choices_request_set (GdmPamExtensionChoiceListRequest *,
                              size_t      i,
                              const char *key,
                              const char *value);

char * gdm_choices_request_ask (pam_handle_t *pamh,
                                GdmPamExtensionChoiceListRequest *);

void gdm_choices_request_free (GdmPamExtensionChoiceListRequest *);
