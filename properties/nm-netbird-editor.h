#ifndef NM_NETBIRD_EDITOR_H
#define NM_NETBIRD_EDITOR_H

#include <NetworkManager.h>
#include <gtk/gtk.h>

G_BEGIN_DECLS

#define NETBIRD_TYPE_EDITOR (netbird_editor_get_type())
G_DECLARE_FINAL_TYPE(NetbirdEditor, netbird_editor, NETBIRD, EDITOR, GObject)

NMVpnEditor *netbird_editor_new(NMConnection *connection, GError **error);

G_END_DECLS

#endif
