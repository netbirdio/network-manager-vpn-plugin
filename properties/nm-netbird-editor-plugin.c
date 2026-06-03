#ifndef _GNU_SOURCE
#define _GNU_SOURCE
#endif

#include "nm-netbird-editor-model.h"

#include <NetworkManager.h>
#include <dlfcn.h>

typedef struct _NetbirdEditorPlugin {
    GObject parent;
} NetbirdEditorPlugin;

typedef struct _NetbirdEditorPluginClass {
    GObjectClass parent;
} NetbirdEditorPluginClass;

typedef NMVpnEditor *(*NetbirdEditorFactory)(NMVpnEditorPlugin *editor_plugin,
                                             NMConnection *connection,
                                             GError **error);

static void netbird_editor_plugin_iface_init(NMVpnEditorPluginInterface *iface);

#define NETBIRD_TYPE_EDITOR_PLUGIN (netbird_editor_plugin_get_type())
GType netbird_editor_plugin_get_type(void);
NMVpnEditorPlugin *nm_vpn_editor_plugin_factory(GError **error);

G_DEFINE_TYPE_EXTENDED(NetbirdEditorPlugin,
                       netbird_editor_plugin,
                       G_TYPE_OBJECT,
                       0,
                       G_IMPLEMENT_INTERFACE(NM_TYPE_VPN_EDITOR_PLUGIN, netbird_editor_plugin_iface_init))

enum {
    PROP_0,
    PROP_NAME,
    PROP_DESCRIPTION,
    PROP_SERVICE,
};

static const char *
editor_module_name(void)
{
    dlerror();
    if (dlsym(RTLD_DEFAULT, "gtk_container_add"))
        return "libnm-vpn-plugin-netbird-editor.so";
    dlerror();

    return "libnm-gtk4-vpn-plugin-netbird-editor.so";
}

static gpointer
load_editor_factory(const char *module_name, GError **error)
{
    static struct {
        gpointer factory;
        void *module;
        char *module_name;
    } cached = { 0 };
    Dl_info plugin_info;
    char *module_dir = NULL;
    char *module_path = NULL;
    const char *load_error;
    gpointer factory;
    void *module;

    if (cached.factory) {
        if (g_strcmp0(cached.module_name, module_name) != 0) {
            g_set_error(error,
                        NM_VPN_PLUGIN_ERROR,
                        NM_VPN_PLUGIN_ERROR_FAILED,
                        "editor module already loaded as %s",
                        cached.module_name);
            return NULL;
        }
        return cached.factory;
    }

    if (g_path_is_absolute(module_name)) {
        module_path = g_strdup(module_name);
    } else {
        if (!dladdr(nm_vpn_editor_plugin_factory, &plugin_info)) {
            load_error = dlerror();
            g_set_error(error,
                        NM_VPN_PLUGIN_ERROR,
                        NM_VPN_PLUGIN_ERROR_FAILED,
                        "unable to get editor plugin path: %s",
                        load_error ? load_error : "unknown error");
            return NULL;
        }

        module_dir = g_path_get_dirname(plugin_info.dli_fname);
        module_path = g_build_filename(module_dir, module_name, NULL);
    }

    module = dlopen(module_path, RTLD_LAZY | RTLD_LOCAL);
    if (!module) {
        load_error = dlerror();
        if (!g_file_test(module_path, G_FILE_TEST_EXISTS)) {
            g_set_error(error,
                        G_FILE_ERROR,
                        G_FILE_ERROR_NOENT,
                        "missing editor plugin file \"%s\"",
                        module_path);
        } else {
            g_set_error(error,
                        NM_VPN_PLUGIN_ERROR,
                        NM_VPN_PLUGIN_ERROR_FAILED,
                        "cannot load editor plugin: %s",
                        load_error ? load_error : "unknown error");
        }
        g_free(module_dir);
        g_free(module_path);
        return NULL;
    }

    dlerror();
    factory = dlsym(module, "nm_vpn_editor_factory_netbird");
    load_error = dlerror();
    if (load_error) {
        g_set_error(error,
                    NM_VPN_PLUGIN_ERROR,
                    NM_VPN_PLUGIN_ERROR_FAILED,
                    "cannot load editor factory from plugin: %s",
                    load_error);
        dlclose(module);
        g_free(module_dir);
        g_free(module_path);
        return NULL;
    }

    cached.factory = factory;
    /* Keep the module loaded; editor GTypes cannot be unloaded safely. */
    cached.module = module;
    cached.module_name = g_strdup(module_name);

    g_free(module_dir);
    g_free(module_path);
    return cached.factory;
}

static void
netbird_editor_plugin_get_property(GObject *object, guint prop_id, GValue *value, GParamSpec *pspec)
{
    (void) object;

    switch (prop_id) {
    case PROP_NAME:
        g_value_set_string(value, "NetBird");
        break;
    case PROP_DESCRIPTION:
        g_value_set_string(value, "NetBird VPN");
        break;
    case PROP_SERVICE:
        g_value_set_string(value, NETBIRD_SERVICE_NAME);
        break;
    default:
        G_OBJECT_WARN_INVALID_PROPERTY_ID(object, prop_id, pspec);
        break;
    }
}

static void
netbird_editor_plugin_class_init(NetbirdEditorPluginClass *klass)
{
    GObjectClass *object_class = G_OBJECT_CLASS(klass);

    object_class->get_property = netbird_editor_plugin_get_property;

    g_object_class_override_property(object_class, PROP_NAME, NM_VPN_EDITOR_PLUGIN_NAME);
    g_object_class_override_property(object_class, PROP_DESCRIPTION, NM_VPN_EDITOR_PLUGIN_DESCRIPTION);
    g_object_class_override_property(object_class, PROP_SERVICE, NM_VPN_EDITOR_PLUGIN_SERVICE);
}

static void
netbird_editor_plugin_init(NetbirdEditorPlugin *plugin)
{
    (void) plugin;
}

static NMVpnEditor *
get_editor(NMVpnEditorPlugin *plugin, NMConnection *connection, GError **error)
{
    NetbirdEditorFactory factory;
    NMVpnEditor *editor;

    g_return_val_if_fail(NM_IS_VPN_EDITOR_PLUGIN(plugin), NULL);
    g_return_val_if_fail(NM_IS_CONNECTION(connection), NULL);
    g_return_val_if_fail(!error || !*error, NULL);

    factory = (NetbirdEditorFactory) load_editor_factory(editor_module_name(), error);
    if (!factory)
        return NULL;

    editor = factory(plugin, connection, error);
    if (!editor) {
        if (error && !*error)
            g_set_error_literal(error,
                                NM_VPN_PLUGIN_ERROR,
                                NM_VPN_PLUGIN_ERROR_FAILED,
                                "unknown error creating editor instance");
        return NULL;
    }

    g_return_val_if_fail(NM_IS_VPN_EDITOR(editor), NULL);
    return editor;
}

static NMVpnEditorPluginCapability
get_capabilities(NMVpnEditorPlugin *plugin)
{
    (void) plugin;
    return NM_VPN_EDITOR_PLUGIN_CAPABILITY_NONE;
}

static void
netbird_editor_plugin_iface_init(NMVpnEditorPluginInterface *iface)
{
    iface->get_editor = get_editor;
    iface->get_capabilities = get_capabilities;
}

NMVpnEditorPlugin *
nm_vpn_editor_plugin_factory(GError **error)
{
    (void) error;
    return NM_VPN_EDITOR_PLUGIN(g_object_new(NETBIRD_TYPE_EDITOR_PLUGIN, NULL));
}
