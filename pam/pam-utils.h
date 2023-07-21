#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <security/pam_modules.h>

char *argv_string_get (const char **argv,
                       unsigned     i);

char *get_user (pam_handle_t *pamh,
                const char   *prompt);

const char *get_module_name (pam_handle_t *pamh);

struct pam_response *send_msg (pam_handle_t *pamh,
                               const char   *msg,
                               int           style);
