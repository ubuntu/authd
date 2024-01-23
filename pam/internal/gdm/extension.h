#include <stdlib.h>
#include <string.h>

// FIXME: Use system one once possible.
#include "extensions/gdm-custom-json-pam-extension.h"

static char pam_extension_environment_block[_POSIX_ARG_MAX];
static char **supported_extensions = NULL;

static inline bool
is_gdm_pam_extension_supported (const char *extension)
{
  return GDM_PAM_EXTENSION_SUPPORTED (extension);
}

static inline void
gdm_extensions_advertise_supported (const char *extensions[],
                                    size_t      n_extensions)
{
  if (supported_extensions)
    {
      for (size_t i = 0; supported_extensions[i] != NULL; ++i)
        free (supported_extensions[i]);

      free (supported_extensions);
    }

  supported_extensions = malloc ((n_extensions + 1) * sizeof (char *));

  for (size_t i = 0; i < n_extensions; ++i)
    supported_extensions[i] = strdup (extensions[i]);
  supported_extensions[n_extensions] = NULL;

  GDM_PAM_EXTENSION_ADVERTISE_SUPPORTED_EXTENSIONS (
    pam_extension_environment_block, supported_extensions);
}

static inline void
gdm_custom_json_request_init (GdmPamExtensionJSONProtocol *request,
                              const char                  *proto_name,
                              unsigned int                 proto_version,
                              const char                  *json)
{
  GDM_PAM_EXTENSION_CUSTOM_JSON_REQUEST_INIT (request, proto_name,
                                              proto_version, json);
}
