#include "nm-netbird-editor-model.h"

#include <string.h>

static const char *const auth_keys[] = {
    NETBIRD_KEY_AUTH,
    "auth-mode",
    "authentication",
    "login-mode",
    NULL,
};

static const char *const setup_key_keys[] = {
    NETBIRD_KEY_SETUP_KEY,
    "setupKey",
    "netbird-setup-key",
    NULL,
};

static const char *const management_url_keys[] = {
    NETBIRD_KEY_MANAGEMENT_URL,
    "managementUrl",
    "netbird-management-url",
    NULL,
};

static const char *const admin_url_keys[] = {
    NETBIRD_KEY_ADMIN_URL,
    "adminURL",
    "netbird-admin-url",
    NULL,
};

static const char *const profile_name_keys[] = {
    "profile-name",
    "profileName",
    "profile",
    "netbird-profile-name",
    NULL,
};

static const char *const username_keys[] = {
    NETBIRD_KEY_USERNAME,
    "user-name",
    "user",
    "netbird-username",
    NULL,
};

static const char *const hint_keys[] = {
    NETBIRD_KEY_HINT,
    "login-hint",
    "sso-hint",
    NULL,
};

static const char *const interface_name_keys[] = {
    NETBIRD_KEY_INTERFACE_NAME,
    "interfaceName",
    "netbird-interface-name",
    NULL,
};

static const char *const hostname_keys[] = {
    NETBIRD_KEY_HOSTNAME,
    "host-name",
    NULL,
};

static const char *const pre_shared_key_keys[] = {
    NETBIRD_KEY_PRE_SHARED_KEY,
    "preshared-key",
    "preSharedKey",
    NULL,
};

GQuark
netbird_editor_error_quark(void)
{
    return g_quark_from_static_string("netbird-editor-error-quark");
}

void
netbird_editor_values_init(NetbirdEditorValues *values)
{
    g_return_if_fail(values != NULL);
    memset(values, 0, sizeof(*values));
    values->auth_mode = g_strdup(NETBIRD_AUTH_SSO);
}

void
netbird_editor_values_clear(NetbirdEditorValues *values)
{
    if (!values)
        return;

    g_clear_pointer(&values->auth_mode, g_free);
    g_clear_pointer(&values->management_url, g_free);
    g_clear_pointer(&values->admin_url, g_free);
    g_clear_pointer(&values->username, g_free);
    g_clear_pointer(&values->hint, g_free);
    g_clear_pointer(&values->interface_name, g_free);
    g_clear_pointer(&values->hostname, g_free);
    g_clear_pointer(&values->setup_key, g_free);
    g_clear_pointer(&values->pre_shared_key, g_free);
}

static char *
dup_trimmed(const char *value)
{
    char *copy;

    if (!value)
        return g_strdup("");

    copy = g_strdup(value);
    g_strstrip(copy);
    return copy;
}

static gboolean
is_blank(const char *value)
{
    char *trimmed;
    gboolean blank;

    trimmed = dup_trimmed(value);
    blank = trimmed[0] == '\0';
    g_free(trimmed);
    return blank;
}

static const char *
vpn_data_first(NMSettingVpn *vpn, const char *const *keys)
{
    guint i;

    if (!vpn)
        return NULL;

    for (i = 0; keys[i]; i++) {
        const char *value = nm_setting_vpn_get_data_item(vpn, keys[i]);
        if (value && value[0])
            return value;
    }
    return NULL;
}

static const char *
vpn_secret_first(NMSettingVpn *vpn, const char *const *keys)
{
    guint i;

    if (!vpn)
        return NULL;

    for (i = 0; keys[i]; i++) {
        const char *value = nm_setting_vpn_get_secret(vpn, keys[i]);
        if (value && value[0])
            return value;
    }
    return NULL;
}

static char *
normalize_auth_mode(const char *value)
{
    char *lower;
    char *normalized;
    char *p;

    lower = dup_trimmed(value);
    for (p = lower; *p; p++) {
        if (*p == '_')
            *p = '-';
        else
            *p = g_ascii_tolower(*p);
    }

    if (lower[0] == '\0')
        normalized = g_strdup(NETBIRD_AUTH_SSO);
    else if (g_strcmp0(lower, "setupkey") == 0 || g_strcmp0(lower, "setup-key") == 0 || g_strcmp0(lower, "key") == 0)
        normalized = g_strdup(NETBIRD_AUTH_SETUP_KEY);
    else if (g_strcmp0(lower, "sso") == 0 || g_strcmp0(lower, "browser") == 0 || g_strcmp0(lower, "interactive") == 0)
        normalized = g_strdup(NETBIRD_AUTH_SSO);
    else if (g_strcmp0(lower, "login") == 0 || g_strcmp0(lower, "force-login") == 0 || g_strcmp0(lower, "reuse") == 0)
        normalized = g_strdup(NETBIRD_AUTH_SSO);
    else
        normalized = g_strdup(lower);

    g_free(lower);
    return normalized;
}

static void
replace_string(char **dst, const char *value)
{
    g_free(*dst);
    *dst = dup_trimmed(value);
}

static void
replace_secret_string(char **dst, const char *value)
{
    g_free(*dst);
    *dst = value ? g_strdup(value) : g_strdup("");
}

void
netbird_editor_values_load(NetbirdEditorValues *values, NMConnection *connection)
{
    NMSettingVpn *vpn;
    char *auth_mode;
    const char *value;

    g_return_if_fail(values != NULL);

    vpn = connection ? nm_connection_get_setting_vpn(connection) : NULL;

    auth_mode = normalize_auth_mode(vpn_data_first(vpn, auth_keys));
    g_free(values->auth_mode);
    if (g_strcmp0(auth_mode, NETBIRD_AUTH_SETUP_KEY) == 0 ||
        g_strcmp0(auth_mode, NETBIRD_AUTH_SSO) == 0)
        values->auth_mode = auth_mode;
    else {
        values->auth_mode = g_strdup(NETBIRD_AUTH_SSO);
        g_free(auth_mode);
    }

    replace_string(&values->management_url, vpn_data_first(vpn, management_url_keys));
    replace_string(&values->admin_url, vpn_data_first(vpn, admin_url_keys));
    replace_string(&values->username, vpn_data_first(vpn, username_keys));
    replace_string(&values->hint, vpn_data_first(vpn, hint_keys));
    replace_string(&values->interface_name, vpn_data_first(vpn, interface_name_keys));
    replace_string(&values->hostname, vpn_data_first(vpn, hostname_keys));

    value = vpn_secret_first(vpn, setup_key_keys);
    if (!value)
        value = vpn_data_first(vpn, setup_key_keys);
    replace_secret_string(&values->setup_key, value);

    value = vpn_secret_first(vpn, pre_shared_key_keys);
    if (!value)
        value = vpn_data_first(vpn, pre_shared_key_keys);
    replace_secret_string(&values->pre_shared_key, value);
}

static gboolean
set_validation_error(GError **error, NetbirdEditorError code, const char *message)
{
    g_set_error_literal(error, NETBIRD_EDITOR_ERROR, code, message);
    return FALSE;
}

static gboolean
is_http_url(const char *value)
{
    char *trimmed;
    char *scheme;
    const char *rest;
    const char *host_end;
    gboolean ok;

    if (is_blank(value))
        return TRUE;

    trimmed = dup_trimmed(value);
    scheme = g_uri_parse_scheme(trimmed);
    if (!scheme) {
        g_free(trimmed);
        return FALSE;
    }

    rest = trimmed + strlen(scheme) + 1;
    ok = (g_ascii_strcasecmp(scheme, "http") == 0 || g_ascii_strcasecmp(scheme, "https") == 0) &&
         g_str_has_prefix(rest, "//");
    if (ok) {
        rest += 2;
        host_end = strpbrk(rest, "/?#");
        if (!host_end)
            host_end = rest + strlen(rest);
        ok = host_end > rest;
        while (ok && rest < host_end) {
            if (g_ascii_isspace(*rest))
                ok = FALSE;
            rest++;
        }
    }

    g_free(scheme);
    g_free(trimmed);
    return ok;
}

static gboolean
valid_interface_name(const char *value)
{
    const char *p;

    if (is_blank(value))
        return TRUE;

    if (strlen(value) > 15)
        return FALSE;

    if (g_strcmp0(value, ".") == 0 || g_strcmp0(value, "..") == 0)
        return FALSE;

    for (p = value; *p; p++) {
        if (*p == '/' || g_ascii_isspace(*p) || g_ascii_iscntrl(*p))
            return FALSE;
    }
    return TRUE;
}

gboolean
netbird_editor_values_validate(const NetbirdEditorValues *values, GError **error)
{
    g_return_val_if_fail(values != NULL, FALSE);

    if (g_strcmp0(values->auth_mode, NETBIRD_AUTH_SETUP_KEY) != 0 &&
        g_strcmp0(values->auth_mode, NETBIRD_AUTH_SSO) != 0)
        return set_validation_error(error, NETBIRD_EDITOR_ERROR_INVALID_AUTH, "auth must be setup-key or sso");

    if (!is_http_url(values->management_url))
        return set_validation_error(error, NETBIRD_EDITOR_ERROR_INVALID_URL, "management-url must be an HTTP or HTTPS URL");

    if (!is_http_url(values->admin_url))
        return set_validation_error(error, NETBIRD_EDITOR_ERROR_INVALID_URL, "admin-url must be an HTTP or HTTPS URL");

    if (!valid_interface_name(values->interface_name))
        return set_validation_error(error,
                                    NETBIRD_EDITOR_ERROR_INVALID_INTERFACE,
                                    "interface-name must be 15 bytes or less and must not be '.'/'..' or contain '/', whitespace, or control characters");

    return TRUE;
}

static NMSettingVpn *
ensure_vpn_setting(NMConnection *connection)
{
    NMSettingVpn *vpn;

    vpn = nm_connection_get_setting_vpn(connection);
    if (vpn)
        return vpn;

    vpn = NM_SETTING_VPN(nm_setting_vpn_new());
    nm_connection_add_setting(connection, NM_SETTING(vpn));
    return vpn;
}

static void
ensure_connection_setting(NMConnection *connection)
{
    NMSettingConnection *setting;

    setting = nm_connection_get_setting_connection(connection);
    if (!setting) {
        setting = NM_SETTING_CONNECTION(nm_setting_connection_new());
        nm_connection_add_setting(connection, NM_SETTING(setting));
    }

    g_object_set(setting, NM_SETTING_CONNECTION_TYPE, NM_SETTING_VPN_SETTING_NAME, NULL);
}

static void
remove_data_keys(NMSettingVpn *vpn, const char *const *keys)
{
    guint i;

    for (i = 0; keys[i]; i++)
        nm_setting_vpn_remove_data_item(vpn, keys[i]);
}

static void
remove_secret_keys(NMSettingVpn *vpn, const char *const *keys)
{
    guint i;

    for (i = 0; keys[i]; i++)
        nm_setting_vpn_remove_secret(vpn, keys[i]);
}

static void
set_data_item(NMSettingVpn *vpn, const char *canonical_key, const char *const *keys, const char *value)
{
    char *trimmed;

    remove_data_keys(vpn, keys);

    trimmed = dup_trimmed(value);
    if (trimmed[0])
        nm_setting_vpn_add_data_item(vpn, canonical_key, trimmed);
    g_free(trimmed);
}

static void
set_secret_item(NMSettingVpn *vpn, const char *canonical_key, const char *const *keys, const char *value)
{
    remove_secret_keys(vpn, keys);
    remove_data_keys(vpn, keys);

    if (!is_blank(value))
        nm_setting_vpn_add_secret(vpn, canonical_key, value);
}

gboolean
netbird_editor_values_save(const NetbirdEditorValues *values, NMConnection *connection, GError **error)
{
    NMSettingVpn *vpn;

    g_return_val_if_fail(values != NULL, FALSE);
    g_return_val_if_fail(connection != NULL, FALSE);

    if (!netbird_editor_values_validate(values, error))
        return FALSE;

    ensure_connection_setting(connection);
    vpn = ensure_vpn_setting(connection);
    g_object_set(vpn, NM_SETTING_VPN_SERVICE_TYPE, NETBIRD_SERVICE_NAME, NULL);

    remove_data_keys(vpn, auth_keys);
    if (g_strcmp0(values->auth_mode, NETBIRD_AUTH_SETUP_KEY) == 0)
        nm_setting_vpn_add_data_item(vpn, NETBIRD_KEY_AUTH, NETBIRD_AUTH_SETUP_KEY);
    else if (g_strcmp0(values->auth_mode, NETBIRD_AUTH_SSO) == 0)
        nm_setting_vpn_add_data_item(vpn, NETBIRD_KEY_AUTH, NETBIRD_AUTH_SSO);

    set_data_item(vpn, NETBIRD_KEY_MANAGEMENT_URL, management_url_keys, values->management_url);
    set_data_item(vpn, NETBIRD_KEY_ADMIN_URL, admin_url_keys, values->admin_url);
    remove_data_keys(vpn, profile_name_keys);
    set_data_item(vpn, NETBIRD_KEY_USERNAME, username_keys, values->username);
    set_data_item(vpn, NETBIRD_KEY_INTERFACE_NAME, interface_name_keys, values->interface_name);
    set_data_item(vpn, NETBIRD_KEY_HOSTNAME, hostname_keys, values->hostname);

    if (g_strcmp0(values->auth_mode, NETBIRD_AUTH_SSO) == 0)
        set_data_item(vpn, NETBIRD_KEY_HINT, hint_keys, values->hint);
    else
        remove_data_keys(vpn, hint_keys);

    if (g_strcmp0(values->auth_mode, NETBIRD_AUTH_SETUP_KEY) == 0)
        set_secret_item(vpn, NETBIRD_KEY_SETUP_KEY, setup_key_keys, values->setup_key);
    else {
        remove_secret_keys(vpn, setup_key_keys);
        remove_data_keys(vpn, setup_key_keys);
    }

    set_secret_item(vpn, NETBIRD_KEY_PRE_SHARED_KEY, pre_shared_key_keys, values->pre_shared_key);
    return TRUE;
}
