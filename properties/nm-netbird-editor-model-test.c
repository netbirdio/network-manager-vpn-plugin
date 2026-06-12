#include "nm-netbird-editor-model.h"

#include <string.h>

static NMSettingVpn *
get_vpn(NMConnection *connection)
{
    return nm_connection_get_setting_vpn(connection);
}

static void
test_empty_connection_saves_defaults(void)
{
    NMConnection *connection = nm_simple_connection_new();
    NetbirdEditorValues values;
    GError *error = NULL;
    NMSettingVpn *vpn;
    NMSettingConnection *s_con;

    netbird_editor_values_init(&values);
    g_assert_true(netbird_editor_values_save(&values, connection, &error));
    g_assert_no_error(error);

    vpn = get_vpn(connection);
    g_assert_nonnull(vpn);
    g_assert_cmpstr(nm_setting_vpn_get_service_type(vpn), ==, NETBIRD_SERVICE_NAME);
    g_assert_cmpstr(nm_setting_vpn_get_data_item(vpn, NETBIRD_KEY_AUTH), ==, NETBIRD_AUTH_SSO);

    s_con = nm_connection_get_setting_connection(connection);
    g_assert_nonnull(s_con);
    g_assert_cmpstr(nm_setting_connection_get_connection_type(s_con), ==, NM_SETTING_VPN_SETTING_NAME);

    netbird_editor_values_clear(&values);
    g_object_unref(connection);
}

static void
test_existing_values_load_and_save_canonical_keys(void)
{
    NMConnection *connection = nm_simple_connection_new();
    NMSettingVpn *vpn = NM_SETTING_VPN(nm_setting_vpn_new());
    NetbirdEditorValues values;
    GError *error = NULL;

    nm_connection_add_setting(connection, NM_SETTING(vpn));
    g_object_set(vpn, NM_SETTING_VPN_SERVICE_TYPE, NETBIRD_SERVICE_NAME, NULL);
    nm_setting_vpn_add_data_item(vpn, "auth-mode", "setupKey");
    nm_setting_vpn_add_data_item(vpn, "managementUrl", " https://api.example.com ");
    nm_setting_vpn_add_data_item(vpn, "profile", " prod ");
    nm_setting_vpn_add_data_item(vpn, "unknown-netbird-key", "keep-me");
    nm_setting_vpn_add_data_item(vpn, "setupKey", "plain-secret");

    netbird_editor_values_init(&values);
    netbird_editor_values_load(&values, connection);
    g_assert_cmpstr(values.auth_mode, ==, NETBIRD_AUTH_SETUP_KEY);
    g_assert_cmpstr(values.management_url, ==, "https://api.example.com");
    g_assert_cmpstr(values.setup_key, ==, "plain-secret");

    g_assert_true(netbird_editor_values_save(&values, connection, &error));
    g_assert_no_error(error);

    g_assert_cmpstr(nm_setting_vpn_get_data_item(vpn, NETBIRD_KEY_AUTH), ==, NETBIRD_AUTH_SETUP_KEY);
    g_assert_cmpstr(nm_setting_vpn_get_data_item(vpn, NETBIRD_KEY_MANAGEMENT_URL), ==, "https://api.example.com");
    g_assert_null(nm_setting_vpn_get_data_item(vpn, "profile"));
    g_assert_cmpstr(nm_setting_vpn_get_data_item(vpn, "unknown-netbird-key"), ==, "keep-me");
    g_assert_null(nm_setting_vpn_get_data_item(vpn, "auth-mode"));
    g_assert_null(nm_setting_vpn_get_data_item(vpn, "setupKey"));
    g_assert_cmpstr(nm_setting_vpn_get_secret(vpn, NETBIRD_KEY_SETUP_KEY), ==, "plain-secret");

    netbird_editor_values_clear(&values);
    g_object_unref(connection);
}

static void
test_legacy_auth_defaults_to_sso(void)
{
    NMConnection *connection = nm_simple_connection_new();
    NMSettingVpn *vpn = NM_SETTING_VPN(nm_setting_vpn_new());
    NetbirdEditorValues values;
    GError *error = NULL;

    nm_connection_add_setting(connection, NM_SETTING(vpn));
    g_object_set(vpn, NM_SETTING_VPN_SERVICE_TYPE, NETBIRD_SERVICE_NAME, NULL);
    nm_setting_vpn_add_data_item(vpn, NETBIRD_KEY_AUTH, NETBIRD_AUTH_LOGIN);

    netbird_editor_values_init(&values);
    netbird_editor_values_load(&values, connection);
    g_assert_cmpstr(values.auth_mode, ==, NETBIRD_AUTH_SSO);

    g_assert_true(netbird_editor_values_save(&values, connection, &error));
    g_assert_no_error(error);
    g_assert_cmpstr(nm_setting_vpn_get_data_item(vpn, NETBIRD_KEY_AUTH), ==, NETBIRD_AUTH_SSO);

    netbird_editor_values_clear(&values);
    g_object_unref(connection);
}

static void
test_sso_saves_hint_and_removes_setup_key(void)
{
    NMConnection *connection = nm_simple_connection_new();
    NetbirdEditorValues values;
    GError *error = NULL;
    NMSettingVpn *vpn;

    netbird_editor_values_init(&values);
    g_free(values.auth_mode);
    values.auth_mode = g_strdup(NETBIRD_AUTH_SSO);
    values.hint = g_strdup(" alice@example.com ");
    values.setup_key = g_strdup("do-not-save");

    g_assert_true(netbird_editor_values_save(&values, connection, &error));
    g_assert_no_error(error);

    vpn = get_vpn(connection);
    g_assert_cmpstr(nm_setting_vpn_get_data_item(vpn, NETBIRD_KEY_AUTH), ==, NETBIRD_AUTH_SSO);
    g_assert_cmpstr(nm_setting_vpn_get_data_item(vpn, NETBIRD_KEY_HINT), ==, "alice@example.com");
    g_assert_null(nm_setting_vpn_get_secret(vpn, NETBIRD_KEY_SETUP_KEY));
    g_assert_null(nm_setting_vpn_get_data_item(vpn, NETBIRD_KEY_SETUP_KEY));

    netbird_editor_values_clear(&values);
    g_object_unref(connection);
}

static void
test_empty_fields_remove_known_keys(void)
{
    NMConnection *connection = nm_simple_connection_new();
    NMSettingVpn *vpn = NM_SETTING_VPN(nm_setting_vpn_new());
    NetbirdEditorValues values;
    GError *error = NULL;

    nm_connection_add_setting(connection, NM_SETTING(vpn));
    g_object_set(vpn, NM_SETTING_VPN_SERVICE_TYPE, NETBIRD_SERVICE_NAME, NULL);
    nm_setting_vpn_add_data_item(vpn, NETBIRD_KEY_MANAGEMENT_URL, "https://api.example.com");
    nm_setting_vpn_add_data_item(vpn, NETBIRD_KEY_INTERFACE_NAME, "wt0");
    nm_setting_vpn_add_secret(vpn, NETBIRD_KEY_PRE_SHARED_KEY, "old-secret");

    netbird_editor_values_init(&values);
    g_assert_true(netbird_editor_values_save(&values, connection, &error));
    g_assert_no_error(error);

    g_assert_null(nm_setting_vpn_get_data_item(vpn, NETBIRD_KEY_MANAGEMENT_URL));
    g_assert_null(nm_setting_vpn_get_data_item(vpn, NETBIRD_KEY_INTERFACE_NAME));
    g_assert_null(nm_setting_vpn_get_secret(vpn, NETBIRD_KEY_PRE_SHARED_KEY));

    netbird_editor_values_clear(&values);
    g_object_unref(connection);
}

static void
test_validation_rejects_invalid_url_and_interface(void)
{
    NetbirdEditorValues values;
    GError *error = NULL;

    netbird_editor_values_init(&values);
    values.management_url = g_strdup("ftp://example.com");
    g_assert_false(netbird_editor_values_validate(&values, &error));
    g_assert_error(error, NETBIRD_EDITOR_ERROR, NETBIRD_EDITOR_ERROR_INVALID_URL);
    g_clear_error(&error);
    g_clear_pointer(&values.management_url, g_free);

    values.interface_name = g_strdup("bad/name");
    g_assert_false(netbird_editor_values_validate(&values, &error));
    g_assert_error(error, NETBIRD_EDITOR_ERROR, NETBIRD_EDITOR_ERROR_INVALID_INTERFACE);
    g_clear_error(&error);

    netbird_editor_values_clear(&values);
}

static void
assert_interface_validation(const char *interface_name, gboolean expected_valid)
{
    NetbirdEditorValues values;
    GError *error = NULL;

    netbird_editor_values_init(&values);
    values.interface_name = g_strdup(interface_name);

    g_assert_cmpint(netbird_editor_values_validate(&values, &error), ==, expected_valid);
    if (expected_valid)
        g_assert_no_error(error);
    else {
        g_assert_error(error, NETBIRD_EDITOR_ERROR, NETBIRD_EDITOR_ERROR_INVALID_INTERFACE);
        g_clear_error(&error);
    }

    netbird_editor_values_clear(&values);
}

static void
test_validation_accepts_valid_interface_names(void)
{
    assert_interface_validation(NULL, TRUE);
    assert_interface_validation("", TRUE);
    assert_interface_validation("wt0", TRUE);
    assert_interface_validation("nb-abcdef123456", TRUE);
}

static void
test_validation_rejects_invalid_interface_names(void)
{
    assert_interface_validation("1234567890123456", FALSE);
    assert_interface_validation("bad\x01" "name", FALSE);
    assert_interface_validation("bad\x7f" "name", FALSE);
    assert_interface_validation(".", FALSE);
    assert_interface_validation("..", FALSE);
}

int
main(int argc, char **argv)
{
    g_test_init(&argc, &argv, NULL);
    g_test_add_func("/netbird-editor/empty-defaults", test_empty_connection_saves_defaults);
    g_test_add_func("/netbird-editor/canonical-save", test_existing_values_load_and_save_canonical_keys);
    g_test_add_func("/netbird-editor/legacy-auth-defaults-to-sso", test_legacy_auth_defaults_to_sso);
    g_test_add_func("/netbird-editor/sso-save", test_sso_saves_hint_and_removes_setup_key);
    g_test_add_func("/netbird-editor/clear-fields", test_empty_fields_remove_known_keys);
    g_test_add_func("/netbird-editor/validation", test_validation_rejects_invalid_url_and_interface);
    g_test_add_func("/netbird-editor/validation-valid-interfaces", test_validation_accepts_valid_interface_names);
    g_test_add_func("/netbird-editor/validation-invalid-interfaces", test_validation_rejects_invalid_interface_names);
    return g_test_run();
}
