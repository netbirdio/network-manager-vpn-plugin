#ifndef NM_NETBIRD_EDITOR_MODEL_H
#define NM_NETBIRD_EDITOR_MODEL_H

#include <NetworkManager.h>
#include <glib.h>

#define NETBIRD_SERVICE_NAME "org.freedesktop.NetworkManager.netbird"

#define NETBIRD_AUTH_REUSE "reuse"
#define NETBIRD_AUTH_LOGIN "login"
#define NETBIRD_AUTH_SETUP_KEY "setup-key"
#define NETBIRD_AUTH_SSO "sso"

#define NETBIRD_KEY_AUTH "auth"
#define NETBIRD_KEY_SETUP_KEY "setup-key"
#define NETBIRD_KEY_MANAGEMENT_URL "management-url"
#define NETBIRD_KEY_ADMIN_URL "admin-url"
#define NETBIRD_KEY_USERNAME "username"
#define NETBIRD_KEY_HINT "hint"
#define NETBIRD_KEY_INTERFACE_NAME "interface-name"
#define NETBIRD_KEY_HOSTNAME "hostname"
#define NETBIRD_KEY_PRE_SHARED_KEY "pre-shared-key"

G_BEGIN_DECLS

typedef enum {
    NETBIRD_EDITOR_ERROR_INVALID_AUTH,
    NETBIRD_EDITOR_ERROR_INVALID_URL,
    NETBIRD_EDITOR_ERROR_INVALID_INTERFACE,
} NetbirdEditorError;

#define NETBIRD_EDITOR_ERROR netbird_editor_error_quark()
GQuark netbird_editor_error_quark(void);

typedef struct {
    char *auth_mode;
    char *management_url;
    char *admin_url;
    char *username;
    char *hint;
    char *interface_name;
    char *hostname;
    char *setup_key;
    char *pre_shared_key;
} NetbirdEditorValues;

void netbird_editor_values_init(NetbirdEditorValues *values);
void netbird_editor_values_clear(NetbirdEditorValues *values);
void netbird_editor_values_load(NetbirdEditorValues *values, NMConnection *connection);
gboolean netbird_editor_values_validate(const NetbirdEditorValues *values, GError **error);
gboolean netbird_editor_values_save(const NetbirdEditorValues *values, NMConnection *connection, GError **error);

G_END_DECLS

#endif
