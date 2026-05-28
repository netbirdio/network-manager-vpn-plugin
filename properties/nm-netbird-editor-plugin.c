#include "nm-netbird-editor.h"
#include "nm-netbird-editor-model.h"

#include <NetworkManager.h>

typedef struct _NetbirdEditorPlugin {
    GObject parent;
} NetbirdEditorPlugin;

typedef struct _NetbirdEditorPluginClass {
    GObjectClass parent;
} NetbirdEditorPluginClass;

static void netbird_editor_plugin_iface_init(NMVpnEditorPluginInterface *iface);

#define NETBIRD_TYPE_EDITOR_PLUGIN (netbird_editor_plugin_get_type())
GType netbird_editor_plugin_get_type(void);

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
    (void) plugin;
    return netbird_editor_new(connection, error);
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
