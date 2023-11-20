/* A simple PAM wrapper for GO based pam modules
 *
 * Copyright (C) 2023 Marco Trevisan
 *
 * SPDX-License-Identifier: LGPL-2.1-or-later
 *
 * This library is free software; you can redistribute it and/or
 * modify it under the terms of the GNU Lesser General Public
 * License as published by the Free Software Foundation; either
 * version 2.1 of the License, or (at your option) any later version.
 *
 * This library is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the GNU
 * Lesser General Public License for more details.
 *
 * You should have received a copy of the GNU Lesser General
 * Public License along with this library; if not, see <http://www.gnu.org/licenses/>.
 *
 * Author: Marco Trevisan <marco.trevisan@canonical.com>
 */

#include <dlfcn.h>
#include <limits.h>
#include <security/pam_modules.h>
#include <security/pam_ext.h>
#include <stddef.h>
#include <stdio.h>
#include <string.h>

/* When a Go shared library is loaded from C, go starts various goroutine
 * (as init() at first) and if the loading code is then performing a fork
 * we end up having an undefined behavior and very likely, deadlocks.
 *
 * To avoid this we need to ensure that the module is called after the loading
 * application has forked, and this for sure has happened when the application
 * calls the PAM functions that a module exposes.
 *
 * As per this, here we re-implement the PAM modules functions and for each
 * action we load the module if it has not been loaded already, otherwise we
 * dload it and we redirect each call to the loaded library.
 */

typedef int (*PamHandler)(pam_handle_t *,
                          int          flags,
                          int          argc,
                          const char **argv);

static void
on_go_module_removed (pam_handle_t *pamh,
                      void         *go_module,
                      int           error_status)
{
  void (*go_pam_cleanup) (void);
  *(void **) (&go_pam_cleanup) = dlsym (go_module, "go_pam_cleanup_module");
  if (go_pam_cleanup)
    go_pam_cleanup ();

  dlclose (go_module);
}

static void *
load_module (pam_handle_t *pamh,
             const char   *module_path)
{
  void *go_module;

  if (pam_get_data (pamh, "go-module", (const void **) &go_module) == PAM_SUCCESS)
    return go_module;

  go_module = dlopen (module_path, RTLD_LAZY);
  if (!go_module)
    return NULL;

  pam_set_data (pamh, "go-module", go_module, on_go_module_removed);

  void (*init_module) (void);
  *(void **) (&init_module) = dlsym (go_module, "go_pam_init_module");
  if (init_module)
    init_module ();

  return go_module;
}

static inline int
call_pam_function (pam_handle_t *pamh,
                   const char   *function,
                   int           flags,
                   int           argc,
                   const char  **argv)
{
  char module_path[PATH_MAX] = {0};
  const char *sub_module;
  PamHandler func;
  void *go_module;

  if (argc < 1)
    {
      pam_error (pamh, "%s: no module provided", function);
      return PAM_MODULE_UNKNOWN;
    }

  sub_module = argv[0];
  argc -= 1;
  argv = (argc == 0) ? NULL : &argv[1];

  strncpy (module_path, sub_module, PATH_MAX - 1);
  go_module = load_module (pamh, module_path);
  if (!go_module)
    {
      pam_error (pamh, "Impossible to load module %s", module_path);
      return PAM_OPEN_ERR;
    }

  *(void **) (&func) = dlsym (go_module, function);
  if (!func)
    {
      pam_error (pamh, "Symbol %s not found in %s", function, module_path);
      return PAM_OPEN_ERR;
    }

  return func (pamh, flags, argc, argv);
}

#define DEFINE_PAM_WRAPPER(name) \
  PAM_EXTERN int \
    (pam_sm_ ## name) (pam_handle_t * pamh, int flags, int argc, const char **argv) \
  { \
    return call_pam_function (pamh, "pam_sm_" #name, flags, argc, argv); \
  }

DEFINE_PAM_WRAPPER (authenticate)
DEFINE_PAM_WRAPPER (chauthtok)
DEFINE_PAM_WRAPPER (close_session)
DEFINE_PAM_WRAPPER (open_session)
DEFINE_PAM_WRAPPER (setcred)
