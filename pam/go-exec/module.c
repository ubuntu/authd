/* A simple PAM wrapper for GO based pam modules
 *
 * Copyright (C) 2024 Canonical Ltd.
 *
 * SPDX-License-Identifier: LGPL-3.0
 *
 * This library is free software; you can redistribute it and/or
 * modify it under the terms of the GNU Lesser General Public
 * License as published by the Free Software Foundation; either
 * version 3 of the License, or (at your option) any later version.
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

#define G_LOG_DOMAIN "authd-pam-exec"

#include <gio/gio.h>
#include <glib/gstdio.h>
#include <security/pam_ext.h>
#include <security/pam_modules.h>

/* If this fails then our assumptions on using the return value as the pam
 * exit status is not valid anymore, so we need to refactor things to use
 * another way to communicate the exit status.
 */
G_STATIC_ASSERT (_PAM_RETURN_VALUES < 255);

G_LOCK_DEFINE_STATIC (exec_module);

/* This struct contains the data of the module, note that it can be shared
 * between different actions when the module has been loaded.
 */
typedef struct
{
  /* Per module-instance data */
  pam_handle_t *pamh;
  GDBusServer  *server;

  /* Per-action data, atomic */
  GPid          child_pid;

  /* Per-action data, protected by the mutex */
  GMainLoop       *loop;
  GDBusConnection *connection;
  GCancellable    *cancellable;
  const char      *current_action;
  guint            child_watch_id;
  gulong           connection_new_id;
  gulong           connection_closed_id;
  guint            object_registered_id;
  int              exit_status;
} ModuleData;

const char *UBUNTU_AUTHD_PAM_OBJECT_NODE =
  "<node>"
  "  <interface name='com.ubuntu.authd.pam'>"
  "    <method name='SetItem'>"
  "      <arg type='i' name='item' direction='in'/>"
  "      <arg type='s' name='value' direction='in'/>"
  "      <arg type='i' name='ret' direction='out'/>"
  "    </method>"
  "    <method name='GetItem'>"
  "      <arg type='i' name='item' direction='in'/>"
  "      <arg type='i' name='status' direction='out'/>"
  "      <arg type='s' name='ret' direction='out'/>"
  "    </method>"
  "    <method name='SetEnv'>"
  "      <arg type='s' name='env' direction='in'/>"
  "      <arg type='s' name='value' direction='in'/>"
  "      <arg type='i' name='ret' direction='out'/>"
  "    </method>"
  "    <method name='UnsetEnv'>"
  "      <arg type='s' name='env' direction='in'/>"
  "      <arg type='i' name='ret' direction='out'/>"
  "    </method>"
  "    <method name='GetEnv'>"
  "      <arg type='s' name='env' direction='in'/>"
  "      <arg type='i' name='status' direction='out'/>"
  "      <arg type='s' name='ret' direction='out'/>"
  "    </method>"
  "    <method name='GetEnvList'>"
  "      <arg type='i' name='ret' direction='out'/>"
  "      <arg type='a{ss}' name='ret' direction='out'/>"
  "    </method>"
  "    <method name='SetData'>"
  "      <arg type='s' name='key' direction='in'/>"
  "      <arg type='v' name='value' direction='in'/>"
  "      <arg type='i' name='ret' direction='out'/>"
  "    </method>"
  "    <method name='UnsetData'>"
  "      <arg type='s' name='key' direction='in'/>"
  "      <arg type='i' name='ret' direction='out'/>"
  "    </method>"
  "    <method name='GetData'>"
  "      <arg type='s' name='key' direction='in'/>"
  "      <arg type='i' name='status' direction='out'/>"
  "      <arg type='v' name='ret' direction='out'/>"
  "    </method>"
  "    <method name='Prompt'>"
  "      <arg type='i' name='style' direction='in'/>"
  "      <arg type='s' name='msg' direction='in'/>"
  "      <arg type='i' name='status' direction='out'/>"
  "      <arg type='s' name='response' direction='out'/>"
  "    </method>"
  "  </interface>"
  "</node>";

#if !GLIB_CHECK_VERSION (2, 76, 0)
/* This is directly imported from newer GLib, it's not needed by 24.04
 * but will be used for earlier LTSs
 */
static inline gboolean
g_clear_fd (int     *fd_ptr,
            GError **error)
{
  int fd = *fd_ptr;

  *fd_ptr = -1;

  if (fd < 0)
    return TRUE;

  /* Suppress "Not available before" warning */
  G_GNUC_BEGIN_IGNORE_DEPRECATIONS
    return g_close (fd, error);
  G_GNUC_END_IGNORE_DEPRECATIONS
}

static inline void
_g_clear_fd_ignore_error (int *fd_ptr)
{
  if (!g_clear_fd (fd_ptr, NULL))
    {
      /* Do nothing: we ignore all errors, except for EBADF which
       * is a programming error, checked for by g_close(). */
    }
}

#define g_autofd _GLIB_CLEANUP (_g_clear_fd_ignore_error)
#endif

G_GNUC_PRINTF (3, 4)
static void
notify_error (pam_handle_t *pamh,
              const char   *action,
              const char   *format,
              ...)
{
  g_autofree char *message = NULL;
  va_list args;

  g_return_if_fail (format != NULL);

  va_start (args, format);
  message = g_strdup_vprintf (format, args);
  va_end (args);

  if (isatty (STDERR_FILENO)) \
    g_debug ("%s: %s", action, message);
  else
    g_warning ("%s: %s", action, message);

  pam_error (pamh, "%s: %s", action, message);
}

static GLogWriterOutput
log_writer (GLogLevelFlags   log_level,
            const GLogField *fields,
            gsize            n_fields,
            gpointer         user_data)
{
  g_autofree char *log = NULL;

  if (g_log_writer_default_would_drop (log_level, G_LOG_DOMAIN))
    return G_LOG_WRITER_HANDLED;

  log = g_log_writer_format_fields (log_level, fields, n_fields,
                                    g_log_writer_supports_color (STDERR_FILENO));

  /* We prefer writing to stderr because loaders such as SSHd may ignore stdout */
  g_printerr ("%s\n", log);
  return G_LOG_WRITER_HANDLED;
}

/* This fake type is meant to be used for cleaning up the per-action values */
typedef ModuleData ActionModuleData;

static void
action_module_data_cleanup (ActionModuleData *module_data)
{
  GDBusServer *server = NULL;
  GPid pid;

  if ((server = g_atomic_pointer_get (&module_data->server)))
    g_clear_signal_handler (&module_data->connection_new_id, server);

  if (module_data->connection)
    {
      g_dbus_connection_unregister_object (module_data->connection,
                                           module_data->object_registered_id);
      g_clear_signal_handler (&module_data->connection_closed_id, module_data->connection);
    }

  g_cancellable_cancel (module_data->cancellable);

  g_log_set_debug_enabled (FALSE);

  g_clear_object (&module_data->cancellable);
  g_clear_object (&module_data->connection);
  g_clear_pointer (&module_data->loop, g_main_loop_unref);
  g_clear_handle_id (&module_data->child_watch_id, g_source_remove);

#if GLIB_CHECK_VERSION (2, 74, 0)
  pid = g_atomic_int_exchange (&module_data->child_pid, 0);
#else
  pid = g_atomic_int_get (&module_data->child_pid);
  g_atomic_int_set (&module_data->child_pid, 0);
#endif
  g_clear_handle_id (&pid, g_spawn_close_pid);

  module_data->current_action = NULL;
}

G_DEFINE_AUTOPTR_CLEANUP_FUNC (ActionModuleData, action_module_data_cleanup)

static void
on_exec_module_removed (pam_handle_t *pamh,
                        void         *data,
                        int           error_status)
{
  g_autoptr(GDBusServer) server = NULL;
  ModuleData *module_data = data;

  action_module_data_cleanup (module_data);

#if GLIB_CHECK_VERSION (2, 74, 0)
  server = g_atomic_pointer_exchange (&module_data->server, NULL);
#else
  server = g_atomic_pointer_get (&module_data->server);
  g_atomic_pointer_set (&module_data->server, NULL);
#endif

  if (server)
    {
      char *tmpdir;

      g_clear_signal_handler (&module_data->connection_new_id, server);
      g_dbus_server_stop (server);

      tmpdir = g_object_get_data (G_OBJECT (server), "tmpdir");
      g_clear_pointer (&tmpdir, g_rmdir);
    }

  g_clear_object (&module_data->connection);
  g_free (module_data);
}

static ModuleData *
setup_shared_module_data (pam_handle_t *pamh)
{
  static const char *module_data_key = "go-exec-module-data";
  ModuleData *module_data = NULL;

  if (pam_get_data (pamh, module_data_key, (const void **) &module_data) == PAM_SUCCESS)
    return module_data;

  module_data = g_new0 (ModuleData, 1);
  module_data->pamh = pamh;
  pam_set_data (pamh, module_data_key, module_data, on_exec_module_removed);

  return module_data;
}

static gboolean
is_debug_logging_enabled ()
{
  const char *debug_messages;

  if (g_log_get_debug_enabled ())
    return TRUE;

  if (!(debug_messages = g_getenv ("G_MESSAGES_DEBUG")))
    return FALSE;

  return g_str_equal (debug_messages, "all") ||
         strstr (debug_messages, G_LOG_DOMAIN);
}

static void
on_child_gone (GPid   pid,
               int    wait_status,
               void * user_data)
{
  g_autoptr(GError) error = NULL;
  ModuleData *module_data = user_data;

  module_data->exit_status = WEXITSTATUS (wait_status);

  g_debug ("Child %" G_PID_FORMAT " exited with exit status %d (%s)", pid,
           module_data->exit_status,
           pam_strerror (NULL, module_data->exit_status));

  if (module_data->connection)
    {
      g_dbus_connection_unregister_object (module_data->connection,
                                           module_data->object_registered_id);

      if (!g_dbus_connection_is_closed (module_data->connection) &&
          !g_dbus_connection_close_sync (module_data->connection,
                                         module_data->cancellable,
                                         &error))
        if (!g_error_matches (error, G_IO_ERROR, G_IO_ERROR_CANCELLED))
          g_warning ("Impossible to close connection: %s", error->message);
    }

  module_data->child_watch_id = 0;

  g_clear_handle_id (&module_data->child_pid, g_spawn_close_pid);
  g_main_loop_quit (module_data->loop);
}

static void
on_other_child_wait (GPid   pid,
                     int    wait_status,
                     void * user_data)
{
  GMainLoop *loop = user_data;

  g_debug ("Done waiting for PID %" G_PID_FORMAT " to close", pid);
  g_main_loop_quit (loop);
}

static void
on_variant_data_removed (pam_handle_t *pamh,
                         void         *data,
                         int           error_status)
{
  g_autoptr(GVariant) G_GNUC_UNUSED variant = g_steal_pointer (&data);
}

static char *
sanitize_variant_key (const char *key)
{
  return g_strdup_printf ("exec-module-variant-%s", key);
}

static void
on_pam_method_call (GDBusConnection       *connection,
                    const char            *sender,
                    const char            *object_path,
                    const char            *interface_name,
                    const char            *method_name,
                    GVariant              *parameters,
                    GDBusMethodInvocation *invocation,
                    void                 * user_data)
{
  ModuleData *module_data = user_data;

  if (is_debug_logging_enabled ())
    {
      g_autofree char *args = g_variant_print (parameters, TRUE);

      g_debug ("%s: called method %s(%s)",
               module_data->current_action, method_name, args);
    }

  if (g_str_equal (method_name, "SetItem"))
    {
      const char *value;
      int item;
      int ret;

      g_variant_get (parameters, "(i&s)", &item, &value);
      ret = pam_set_item (module_data->pamh, item, value);
      g_dbus_method_invocation_return_value (invocation, g_variant_new ("(i)", ret));
    }
  else if (g_str_equal (method_name, "GetItem"))
    {
      int item;
      int ret;
      const void *value;

      g_variant_get (parameters, "(i)", &item);
      ret = pam_get_item (module_data->pamh, item, &value);
      value = value ? value : "";
      g_dbus_method_invocation_return_value (invocation,
                                             g_variant_new ("(is)", ret, value));
    }
  else if (g_str_equal (method_name, "SetEnv"))
    {
      const char *env;
      const char *value;
      int ret;
      g_autofree char *name_value = NULL;

      g_variant_get (parameters, "(&s&s)", &env, &value);
      name_value = g_strconcat (env, "=", value, NULL);
      ret = pam_putenv (module_data->pamh, name_value);

      g_dbus_method_invocation_return_value (invocation, g_variant_new ("(i)", ret));
      g_debug ("We have the env set?!? %s", pam_getenv (module_data->pamh, env));
    }
  else if (g_str_equal (method_name, "UnsetEnv"))
    {
      const char *env;
      int ret;

      g_variant_get (parameters, "(&s)", &env);
      if (strchr (env, '='))
        {
          g_dbus_method_invocation_return_error (invocation,
                                                 G_DBUS_ERROR, G_DBUS_ERROR_INVALID_ARGS,
                                                 "Invalid char found on env %s", env);
          return;
        }

      ret = pam_putenv (module_data->pamh, env);
      g_dbus_method_invocation_return_value (invocation,
                                             g_variant_new ("(i)", ret));
    }
  else if (g_str_equal (method_name, "GetEnv"))
    {
      const char *env;
      const char *value;

      g_variant_get (parameters, "(&s)", &env);
      value = pam_getenv (module_data->pamh, env);
      value = value ? value : "";

      g_dbus_method_invocation_return_value (invocation,
                                             g_variant_new ("(is)",
                                                            PAM_SUCCESS, value));
    }
  else if (g_str_equal (method_name, "GetEnvList"))
    {
      g_auto(GStrv) env_list = NULL;
      g_auto(GVariantBuilder) dict_builder =
        G_VARIANT_BUILDER_INIT (G_VARIANT_TYPE ("a{ss}"));
      int ret = PAM_SUCCESS;

      env_list = pam_getenvlist (module_data->pamh);

      if (!env_list)
        ret = PAM_BUF_ERR;

      for (int i = 0; env_list && env_list[i]; ++i)
        {
          g_auto(GStrv) env_pair = g_strsplit (env_list[i], "=", 2);

          if (g_strv_length (env_pair) != 2)
            continue;

          g_variant_builder_add (&dict_builder, "{ss}", env_pair[0], env_pair[1]);
        }

      g_dbus_method_invocation_return_value (invocation,
                                             g_variant_new ("(ia{ss})",
                                                            ret, &dict_builder));
    }
  else if (g_str_equal (method_name, "SetData"))
    {
      g_autofree char *variant_key = NULL;
      const char *key;
      GVariant *variant;
      int ret;

      g_variant_get (parameters, "(&sv)", &key, &variant);
      variant_key = sanitize_variant_key (key);
      ret = pam_set_data (module_data->pamh, variant_key, variant, on_variant_data_removed);
      g_dbus_method_invocation_return_value (invocation, g_variant_new ("(i)", ret));
    }
  else if (g_str_equal (method_name, "UnsetData"))
    {
      g_autofree char *variant_key = NULL;
      const char *key;
      int ret;

      g_variant_get (parameters, "(&s)", &key);
      variant_key = sanitize_variant_key (key);
      ret = pam_set_data (module_data->pamh, variant_key, NULL, NULL);
      g_dbus_method_invocation_return_value (invocation,
                                             g_variant_new ("(i)", ret));
    }
  else if (g_str_equal (method_name, "GetData"))
    {
      g_autofree char *variant_key = NULL;
      GVariant *variant = NULL;
      const char *key;
      int ret;

      g_variant_get (parameters, "(&s)", &key);
      variant_key = sanitize_variant_key (key);
      ret = pam_get_data (module_data->pamh, variant_key, (const void **) &variant);

      if (!variant)
        {
          /* If the data is NULL, let's ensure we mark this as an error, and
           * we return some fake "mv" value as string since go-side can't
           * properly handle maybe types.
           */
          g_autoptr(GVariant) maybe_variant = NULL;

          maybe_variant = g_variant_new ("v", g_variant_new_maybe (G_VARIANT_TYPE_VARIANT, NULL));
          variant = g_variant_new_take_string (g_variant_print (maybe_variant, TRUE));

          ret = PAM_NO_MODULE_DATA;
        }

      g_dbus_method_invocation_return_value (invocation,
                                             g_variant_new ("(iv)", ret, variant));
    }
  else if (g_str_equal (method_name, "Prompt"))
    {
      g_autofree char *response = NULL;
      const char *prompt;
      int style;
      int ret;

      g_variant_get (parameters, "(i&s)", &style, &prompt);

      ret = pam_prompt (module_data->pamh, style, &response, "%s", prompt);
      g_dbus_method_invocation_return_value (invocation,
                                             g_variant_new ("(is)", ret,
                                                            response ? response : ""));
    }
  else
    {
      g_dbus_method_invocation_return_error (invocation, G_DBUS_ERROR,
                                             G_DBUS_ERROR_UNKNOWN_METHOD,
                                             "No method implementation for %s",
                                             method_name);
    }
}

static const GDBusInterfaceVTable pam_interface_vtable = {
  on_pam_method_call,
  NULL,  /* get_property */
  NULL,  /* set_property */
  { 0 }
};

static void
on_connection_closed (GDBusConnection *connection,
                      gboolean         remote_peer_vanished,
                      GError          *error,
                      ModuleData      *module_data)
{
  g_debug ("Connection closed %s", g_dbus_connection_get_guid (connection));

  if (!module_data->connection)
    return;

  g_assert (module_data->connection == connection);

  if (module_data->object_registered_id)
    {
      g_dbus_connection_unregister_object (connection, module_data->object_registered_id);
      module_data->object_registered_id = 0;
    }

  module_data->connection = NULL;
}

static gboolean
on_new_connection (G_GNUC_UNUSED GDBusServer *server,
                   GDBusConnection           *connection,
                   gpointer                   user_data)
{
  g_autoptr(GDBusNodeInfo) node = NULL;
  g_autoptr(GError) error = NULL;
  ModuleData *module_data = user_data;
  GCredentials *credentials = g_dbus_connection_get_peer_credentials (connection);
  pid_t client_pid;

  if (module_data->connection)
    {
      notify_error (module_data->pamh, module_data->current_action,
                    "Another client is already using this connection");
      return FALSE;
    }

  if (!G_IS_CREDENTIALS (credentials))
    {
      notify_error (module_data->pamh, module_data->current_action,
                    "Impossible to get credentials, refusing the connection...");
      return FALSE;
    }

  if ((client_pid = g_credentials_get_unix_pid (credentials, &error)) == -1)
    {
      notify_error (module_data->pamh, module_data->current_action,
                    "Impossible to get client PID (%s), refusing the connection...",
                    error->message);
      return FALSE;
    }

  if (client_pid != g_atomic_int_get (&module_data->child_pid) && client_pid != getpid ())
    {
#ifndef AUTHD_TEST_MODULE
      notify_error (module_data->pamh, module_data->current_action,
                    "Child PID is not matching the expected one");
      return FALSE;
#else /*if defined (AUTHD_TEST_MODULE) */
      /* When testing under go it may happen to receive the request from a child
       * of the process we've started. We may do some deeper check here, but
       * it's not big deal when testing, since we're already quite confident
       * we're allowed.
       */
      g_debug ("Client pid PID %" G_PID_FORMAT " is not the expected %" G_PID_FORMAT,
               client_pid, g_atomic_int_get (&module_data->child_pid));
#endif
    }

  node = g_dbus_node_info_new_for_xml (UBUNTU_AUTHD_PAM_OBJECT_NODE, &error);
  if (!node)
    {
      notify_error (module_data->pamh, module_data->current_action,
                    "Can't create node: %s", error->message);
      return FALSE;
    }

  g_debug ("Accepting connection from PID %" G_PID_FORMAT " on connection %s",
           client_pid, g_dbus_connection_get_guid (connection));

  /* export an object */
  error = NULL;
  module_data->object_registered_id =
    g_dbus_connection_register_object (connection,
                                       "/com/ubuntu/authd/pam",
                                       node->interfaces[0],
                                       &pam_interface_vtable,
                                       module_data,
                                       NULL,
                                       &error);

  /* Accepts the connection */
  module_data->connection = g_object_ref (connection);

  module_data->connection_closed_id =
    g_signal_connect (module_data->connection, "closed",
                      G_CALLBACK (on_connection_closed), module_data);

  return TRUE;
}

static GDBusServer *
setup_dbus_server (ModuleData *module_data,
                   const char *action,
                   GError    **error)
{
  GDBusServer *server = NULL;
  g_autofree char *escaped = NULL;
  g_autofree char *server_addr = NULL;
  g_autofree char *guid = NULL;
  g_autofree char *tmpdir = NULL;

  /* This pointer is used as a semaphore, so accessing to server-related stuff
   * does not need further atomic checks.
   */
  if ((server = g_atomic_pointer_get (&module_data->server)))
    return server;

  tmpdir = g_dir_make_tmp ("authd-pam-server-XXXXXX", error);
  if (tmpdir == NULL)
    {
      int errsv = errno;
      g_set_error_literal (error, G_IO_ERROR, g_io_error_from_errno (errsv),
                           g_strerror (errsv));
      return NULL;
    }

  escaped = g_dbus_address_escape_value (tmpdir);
  server_addr = g_strdup_printf ("unix:tmpdir=%s", escaped);
  guid = g_dbus_generate_guid ();

  g_debug ("Setting up connection at %s (%s)", server_addr, guid);
  server = g_dbus_server_new_sync (server_addr,
                                   G_DBUS_SERVER_FLAGS_AUTHENTICATION_REQUIRE_SAME_USER,
                                   guid,
                                   NULL,
                                   module_data->cancellable,
                                   error);
  if (server == NULL)
    return NULL;

  g_object_set_data_full (G_OBJECT (server), "tmpdir",
                          g_steal_pointer (&tmpdir), g_free);
  g_dbus_server_start (server);

  g_debug ("Server started, connectable address %s",
           g_dbus_server_get_client_address (server));

  return server;
}

static int
dup_fd_checked (int fd, GError **error)
{
  int new_fd = dup (fd);

  if (new_fd < 0)
    {
      int errsv = errno;
      g_set_error_literal (error, G_IO_ERROR, g_io_error_from_errno (errsv),
                           g_strerror (errsv));
    }

  return new_fd;
}

static gboolean
handle_module_options (int argc, const
                       char **argv,
                       GPtrArray **out_args,
                       char ***out_env_variables,
                       GError **error)
{
  g_autoptr(GOptionContext) options_context = NULL;
  g_autoptr(GStrvBuilder) strv_builder = NULL;
  g_autoptr(GPtrArray) args = NULL;
  g_auto(GStrv) args_strv = NULL;
  g_auto(GStrv) env_variables = NULL;
  gboolean debug_enabled = FALSE;

  const GOptionEntry options_entries[] = {
    { "exec-env", 0, 0, G_OPTION_ARG_STRING_ARRAY, &env_variables, NULL, NULL },
    { "exec-debug", 0, 0, G_OPTION_ARG_NONE, &debug_enabled, NULL, NULL },
    G_OPTION_ENTRY_NULL
  };

  strv_builder = g_strv_builder_new ();
  /* We temporary add a fake item as first one, since the option parser ignores
   * it, since normally it's just the program name */
  g_strv_builder_add (strv_builder, "pam-go-exec-module");
  for (int i = 0; i < argc; ++i)
    g_strv_builder_add (strv_builder, argv[i]);

  options_context = g_option_context_new ("ARGS...");
  g_option_context_set_ignore_unknown_options (options_context, TRUE);
  g_option_context_set_help_enabled (options_context, FALSE);
  g_option_context_add_main_entries (options_context, options_entries, NULL);

  args_strv = g_strv_builder_end (strv_builder);
  if (!g_option_context_parse_strv (options_context, &args_strv, error))
    return FALSE;

  /* We can now remove the first element that was added */
  argc = g_strv_length (args_strv);
  args = g_ptr_array_new_full (argc - 1, g_free);
  for (int i = 1; i < argc; ++i)
    {
      g_autofree char *arg = g_steal_pointer (&args_strv[i]);

      if (!g_str_equal (arg, "--"))
        g_ptr_array_add (args, g_steal_pointer (&arg));
    }

  if (out_args)
    *out_args = g_steal_pointer (&args);

  if (out_env_variables)
    *out_env_variables = g_steal_pointer (&env_variables);

  g_log_set_debug_enabled (debug_enabled);

  return TRUE;
}

static inline int
do_pam_action (pam_handle_t *pamh,
               const char   *action,
               int           flags,
               int           argc,
               const char  **argv)
{
  g_autoptr(GMutexLocker) G_GNUC_UNUSED locker = NULL;
  g_autoptr(ActionModuleData) module_data = NULL;
  g_autoptr(GError) error = NULL;
  g_autoptr(GPtrArray) envp = NULL;
  g_autoptr(GPtrArray) args = NULL;
  g_autoptr(GDBusServer) server = NULL;
  g_auto(GStrv) env_variables = NULL;
  g_autofree char *exe = NULL;
  g_autofd int stdin_fd = -1;
  g_autofd int stdout_fd = -1;
  g_autofd int stderr_fd = -1;
  static gsize logger_set = FALSE;
  gboolean interactive_mode;
  GPid child_pid;

  if (g_once_init_enter (&logger_set))
    {
      g_log_set_writer_func (log_writer, NULL, NULL);
      g_once_init_leave (&logger_set, TRUE);
    }

  g_debug ("Starting %s", action);

  if (!handle_module_options (argc, argv, &args, &env_variables, &error))
    {
      notify_error (pamh, action, "impossible to parse arguments: %s", error->message);
      return PAM_SYSTEM_ERR;
    }

  if (is_debug_logging_enabled ())
    {
      g_autoptr(GString) str_args = g_string_new (NULL);

      for (int i = 0; i < argc; ++i)
        {
          g_string_append_printf (str_args, "'%s'", argv[i]);

          if (i < argc - 1)
            g_string_append_c (str_args, ' ');
        }

      g_debug ("Called with arguments: %s", str_args->str);
    }

  module_data = setup_shared_module_data (pamh);
  if (module_data == NULL)
    {
      notify_error (pamh, action, "can't create module data");
      return PAM_SYSTEM_ERR;
    }

  if (!args || args->len < 1)
    {
      notify_error (pamh, action, "no executable provided");
      return PAM_MODULE_UNKNOWN;
    }

  exe = g_ptr_array_steal_index (args, 0);

  if (!exe || *exe == '\0')
    {
      notify_error (pamh, action, "no valid module name provided");
      return PAM_MODULE_UNKNOWN;
    }

  if (!g_file_test (exe, G_FILE_TEST_IS_EXECUTABLE))
    {
      notify_error (pamh, action, "Impossible to use %s as PAM executable", exe);
      return PAM_MODULE_UNKNOWN;
    }

  child_pid = g_atomic_int_get (&module_data->child_pid);
  if (G_UNLIKELY (child_pid) != 0)
    {
      /* Unlikely to happen, but if another action PID is still active, let's
       * wait for it before proceeding.
       */
      g_autoptr(GMainLoop) loop = g_main_loop_new (NULL, FALSE);

      g_debug ("Another client with PID %"G_PID_FORMAT
               " is still running, let's wait for it", child_pid);
      g_child_watch_add (child_pid, on_other_child_wait, loop);
      g_main_loop_run (loop);
    }

  server = setup_dbus_server (module_data, action, &error);
  if (!server)
    {
      notify_error (pamh, action, "can't create DBus connection: %s", error->message);
      return PAM_SYSTEM_ERR;
    }

  g_atomic_pointer_set (&module_data->server, g_object_ref (server));

  locker = g_mutex_locker_new (&G_LOCK_NAME (exec_module));
  g_assert (g_atomic_int_get (&module_data->child_pid) == 0);

  g_assert (module_data->current_action == NULL);
  module_data->current_action = action;

  g_assert (module_data->cancellable == NULL);
  module_data->cancellable = g_cancellable_new ();

  interactive_mode = isatty (STDIN_FILENO);

  if (interactive_mode)
    {
      if ((stdin_fd = dup_fd_checked (STDIN_FILENO, &error)) < 0)
        {
          notify_error (pamh, action, "can't duplicate stdin file descriptor: %s",
                        error->message);
          return PAM_SYSTEM_ERR;
        }

      if ((stdout_fd = dup_fd_checked (STDOUT_FILENO, &error)) < 0)
        {
          notify_error (pamh, action, "can't duplicate stdout file descriptor: %s",
                        error->message);
          return PAM_SYSTEM_ERR;
        }

      if ((stderr_fd = dup_fd_checked (STDERR_FILENO, &error)) < 0)
        {
          notify_error (pamh, action, "can't duplicate stderr file descriptor: %s",
                        error->message);
          return PAM_SYSTEM_ERR;
        }
    }

  module_data->connection_new_id =
    g_signal_connect (server, "new-connection",
                      G_CALLBACK (on_new_connection), module_data);

  while (!g_dbus_server_is_active (server))
    g_thread_yield ();

  envp = g_ptr_array_new_full (2, g_free);
  if (interactive_mode)
    g_ptr_array_add (envp, g_strdup_printf ("TERM=%s", g_getenv ("TERM")));
  for (int i = 0; env_variables && env_variables[i]; ++i)
    g_ptr_array_add (envp, g_strdup (env_variables[i]));
  /* FIXME: use g_ptr_array_new_null_terminated when we can use newer GLib. */
  g_ptr_array_add (envp, NULL);

  int idx = 0;
  g_ptr_array_insert (args, idx++, g_strdup (exe));
  g_ptr_array_insert (args, idx++, g_strdup ("-flags"));
  g_ptr_array_insert (args, idx++, g_strdup_printf ("%d", flags));
  g_ptr_array_insert (args, idx++, g_strdup ("-server-address"));
  g_ptr_array_insert (args, idx++, g_strdup (g_dbus_server_get_client_address (server)));
  g_ptr_array_insert (args, idx++, g_strdup (action));
  /* FIXME: use g_ptr_array_new_null_terminated when we can use newer GLib. */
  g_ptr_array_add (args, NULL);

  if (is_debug_logging_enabled ())
    {
      g_autofree char *exec_str_args = g_strjoinv (" ", (char **) args->pdata);

      g_debug ("Launching '%s'", exec_str_args);
    }

  if (!g_spawn_async_with_fds (NULL,
                               (char **) args->pdata,
                               (GStrv) envp->pdata,
                               G_SPAWN_DO_NOT_REAP_CHILD,
                               NULL, NULL, /* Child setup */
                               &child_pid,
                               stdin_fd,
                               stdout_fd,
                               stderr_fd,
                               &error))
    {
      notify_error (pamh, action, "can't launch %s: %s", exe, error->message);
      return PAM_SYSTEM_ERR;
    }

  g_debug ("Launched child %"G_PID_FORMAT, child_pid);
  g_atomic_int_set (&module_data->child_pid, child_pid);

  module_data->loop = g_main_loop_new (NULL, FALSE);
  module_data->child_watch_id =
    g_child_watch_add_full (G_PRIORITY_HIGH, child_pid,
                            on_child_gone, module_data, NULL);

  g_main_loop_run (module_data->loop);

  if (module_data->exit_status >= _PAM_RETURN_VALUES)
    return PAM_SYSTEM_ERR;

  return module_data->exit_status;
}

#define DEFINE_PAM_WRAPPER(name) \
  PAM_EXTERN int \
    (pam_sm_ ## name) (pam_handle_t * pamh, int flags, int argc, const char **argv) \
  { \
    return do_pam_action (pamh, #name, flags, argc, argv); \
  }

DEFINE_PAM_WRAPPER (acct_mgmt)
DEFINE_PAM_WRAPPER (authenticate)
DEFINE_PAM_WRAPPER (chauthtok)
DEFINE_PAM_WRAPPER (close_session)
DEFINE_PAM_WRAPPER (open_session)
DEFINE_PAM_WRAPPER (setcred)
