#include "nm-netbird-editor-model.h"

#include <NetworkManager.h>

static void
test_plugin_loads_from_file(void)
{
    const char *path;
    GError *error = NULL;
    NMVpnEditorPlugin *plugin;
    char *name = NULL;
    char *service = NULL;

    path = g_getenv("NETBIRD_EDITOR_PLUGIN_PATH");
    g_assert_nonnull(path);
    g_assert_true(g_path_is_absolute(path));

    plugin = nm_vpn_editor_plugin_load_from_file(path, NETBIRD_SERVICE_NAME, -1, NULL, NULL, &error);
    g_assert_no_error(error);
    g_assert_nonnull(plugin);

    g_object_get(plugin,
                 NM_VPN_EDITOR_PLUGIN_NAME,
                 &name,
                 NM_VPN_EDITOR_PLUGIN_SERVICE,
                 &service,
                 NULL);

    g_assert_cmpstr(name, ==, "NetBird");
    g_assert_cmpstr(service, ==, NETBIRD_SERVICE_NAME);

    g_free(name);
    g_free(service);
    g_object_unref(plugin);
}

int
main(int argc, char **argv)
{
    g_test_init(&argc, &argv, NULL);
    g_test_add_func("/netbird-editor/plugin-load", test_plugin_loads_from_file);
    return g_test_run();
}
