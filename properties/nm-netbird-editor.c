#include "nm-netbird-editor.h"

#include "nm-netbird-editor-model.h"

struct _NetbirdEditor {
    GObject parent;

    GtkWidget *widget;
    GtkWidget *auth_combo;
    GtkWidget *setup_key_section;
    GtkWidget *sso_section;
    GtkWidget *management_url_entry;
    GtkWidget *admin_url_entry;
    GtkWidget *profile_name_entry;
    GtkWidget *username_entry;
    GtkWidget *setup_key_entry;
    GtkWidget *hint_entry;
    GtkWidget *interface_name_entry;
    GtkWidget *hostname_entry;
    GtkWidget *pre_shared_key_entry;
    gboolean loading;
};

static void netbird_editor_iface_init(NMVpnEditorInterface *iface);

G_DEFINE_TYPE_EXTENDED(NetbirdEditor,
                       netbird_editor,
                       G_TYPE_OBJECT,
                       0,
                       G_IMPLEMENT_INTERFACE(NM_TYPE_VPN_EDITOR, netbird_editor_iface_init))

static GtkWidget *
new_section(const char *title)
{
    GtkWidget *frame;
    GtkWidget *grid;

    frame = gtk_frame_new(title);
    gtk_widget_set_hexpand(frame, TRUE);

    grid = gtk_grid_new();
    gtk_grid_set_row_spacing(GTK_GRID(grid), 8);
    gtk_grid_set_column_spacing(GTK_GRID(grid), 12);
    gtk_container_set_border_width(GTK_CONTAINER(grid), 12);
    gtk_container_add(GTK_CONTAINER(frame), grid);

    return frame;
}

static GtkWidget *
section_grid(GtkWidget *section)
{
    return gtk_bin_get_child(GTK_BIN(section));
}

static GtkWidget *
new_label(const char *text)
{
    GtkWidget *label;

    label = gtk_label_new(text);
    gtk_widget_set_halign(label, GTK_ALIGN_START);
    gtk_widget_set_valign(label, GTK_ALIGN_CENTER);
    return label;
}

static GtkWidget *
new_entry(void)
{
    GtkWidget *entry;

    entry = gtk_entry_new();
    gtk_widget_set_hexpand(entry, TRUE);
    return entry;
}

static void
attach_row(GtkWidget *grid, int row, const char *label, GtkWidget *input)
{
    gtk_grid_attach(GTK_GRID(grid), new_label(label), 0, row, 1, 1);
    gtk_grid_attach(GTK_GRID(grid), input, 1, row, 1, 1);
}

static void
emit_changed(NetbirdEditor *self)
{
    if (self->loading)
        return;

    g_signal_emit_by_name(self, "changed");
}

static void
update_visible_sections(NetbirdEditor *self)
{
    const char *auth_mode;

    auth_mode = gtk_combo_box_get_active_id(GTK_COMBO_BOX(self->auth_combo));
    gtk_widget_set_visible(self->setup_key_section, g_strcmp0(auth_mode, NETBIRD_AUTH_SETUP_KEY) == 0);
    gtk_widget_set_visible(self->sso_section, g_strcmp0(auth_mode, NETBIRD_AUTH_SSO) == 0);
}

static void
widget_changed_cb(GtkWidget *widget, gpointer user_data)
{
    NetbirdEditor *self = NETBIRD_EDITOR(user_data);

    (void) widget;
    update_visible_sections(self);
    emit_changed(self);
}

static void
connect_changed(NetbirdEditor *self, GtkWidget *widget)
{
    g_signal_connect(widget, "changed", G_CALLBACK(widget_changed_cb), self);
}

static void
set_entry_text(GtkWidget *entry, const char *value)
{
    gtk_entry_set_text(GTK_ENTRY(entry), value ? value : "");
}

static char *
entry_text(GtkWidget *entry)
{
    return g_strdup(gtk_entry_get_text(GTK_ENTRY(entry)));
}

static char *
trimmed_entry_text(GtkWidget *entry)
{
    char *value;

    value = entry_text(entry);
    g_strstrip(value);
    return value;
}

static void
fill_values_from_ui(NetbirdEditor *self, NetbirdEditorValues *values)
{
    const char *auth_mode;

    netbird_editor_values_init(values);

    auth_mode = gtk_combo_box_get_active_id(GTK_COMBO_BOX(self->auth_combo));
    g_free(values->auth_mode);
    values->auth_mode = g_strdup(auth_mode ? auth_mode : NETBIRD_AUTH_REUSE);

    values->management_url = trimmed_entry_text(self->management_url_entry);
    values->admin_url = trimmed_entry_text(self->admin_url_entry);
    values->profile_name = trimmed_entry_text(self->profile_name_entry);
    values->username = trimmed_entry_text(self->username_entry);
    values->hint = trimmed_entry_text(self->hint_entry);
    values->interface_name = trimmed_entry_text(self->interface_name_entry);
    values->hostname = trimmed_entry_text(self->hostname_entry);
    values->setup_key = entry_text(self->setup_key_entry);
    values->pre_shared_key = entry_text(self->pre_shared_key_entry);
}

static void
load_values_into_ui(NetbirdEditor *self, const NetbirdEditorValues *values)
{
    self->loading = TRUE;

    if (!gtk_combo_box_set_active_id(GTK_COMBO_BOX(self->auth_combo), values->auth_mode))
        gtk_combo_box_set_active_id(GTK_COMBO_BOX(self->auth_combo), NETBIRD_AUTH_REUSE);

    set_entry_text(self->management_url_entry, values->management_url);
    set_entry_text(self->admin_url_entry, values->admin_url);
    set_entry_text(self->profile_name_entry, values->profile_name);
    set_entry_text(self->username_entry, values->username);
    set_entry_text(self->hint_entry, values->hint);
    set_entry_text(self->interface_name_entry, values->interface_name);
    set_entry_text(self->hostname_entry, values->hostname);
    set_entry_text(self->setup_key_entry, values->setup_key);
    set_entry_text(self->pre_shared_key_entry, values->pre_shared_key);

    update_visible_sections(self);
    self->loading = FALSE;
}

static void
build_main_section(NetbirdEditor *self)
{
    GtkWidget *section;
    GtkWidget *grid;

    section = new_section("NetBird");
    grid = section_grid(section);

    self->auth_combo = gtk_combo_box_text_new();
    gtk_combo_box_text_append(GTK_COMBO_BOX_TEXT(self->auth_combo), NETBIRD_AUTH_REUSE, "Reuse existing daemon auth");
    gtk_combo_box_text_append(GTK_COMBO_BOX_TEXT(self->auth_combo), NETBIRD_AUTH_LOGIN, "Force daemon login");
    gtk_combo_box_text_append(GTK_COMBO_BOX_TEXT(self->auth_combo), NETBIRD_AUTH_SETUP_KEY, "Setup key");
    gtk_combo_box_text_append(GTK_COMBO_BOX_TEXT(self->auth_combo), NETBIRD_AUTH_SSO, "SSO");
    gtk_widget_set_hexpand(self->auth_combo, TRUE);

    self->management_url_entry = new_entry();
    gtk_entry_set_placeholder_text(GTK_ENTRY(self->management_url_entry), "https://api.netbird.io");
    self->admin_url_entry = new_entry();
    gtk_entry_set_placeholder_text(GTK_ENTRY(self->admin_url_entry), "https://app.netbird.io");
    self->profile_name_entry = new_entry();
    self->username_entry = new_entry();

    attach_row(grid, 0, "Auth mode", self->auth_combo);
    attach_row(grid, 1, "Management URL", self->management_url_entry);
    attach_row(grid, 2, "Admin URL", self->admin_url_entry);
    attach_row(grid, 3, "Profile name", self->profile_name_entry);
    attach_row(grid, 4, "Username", self->username_entry);

    gtk_box_pack_start(GTK_BOX(self->widget), section, FALSE, FALSE, 0);
}

static void
build_setup_key_section(NetbirdEditor *self)
{
    GtkWidget *grid;

    self->setup_key_section = new_section("Setup key");
    grid = section_grid(self->setup_key_section);

    self->setup_key_entry = new_entry();
    gtk_entry_set_visibility(GTK_ENTRY(self->setup_key_entry), FALSE);
    gtk_entry_set_input_purpose(GTK_ENTRY(self->setup_key_entry), GTK_INPUT_PURPOSE_PASSWORD);
    attach_row(grid, 0, "Setup key", self->setup_key_entry);

    gtk_box_pack_start(GTK_BOX(self->widget), self->setup_key_section, FALSE, FALSE, 0);
}

static void
build_sso_section(NetbirdEditor *self)
{
    GtkWidget *grid;

    self->sso_section = new_section("SSO");
    grid = section_grid(self->sso_section);

    self->hint_entry = new_entry();
    gtk_entry_set_placeholder_text(GTK_ENTRY(self->hint_entry), "alice@example.com");
    attach_row(grid, 0, "Login hint", self->hint_entry);

    gtk_box_pack_start(GTK_BOX(self->widget), self->sso_section, FALSE, FALSE, 0);
}

static void
build_advanced_section(NetbirdEditor *self)
{
    GtkWidget *section;
    GtkWidget *grid;

    section = new_section("Advanced");
    grid = section_grid(section);

    self->interface_name_entry = new_entry();
    gtk_entry_set_placeholder_text(GTK_ENTRY(self->interface_name_entry), "wt0");
    self->hostname_entry = new_entry();
    self->pre_shared_key_entry = new_entry();
    gtk_entry_set_visibility(GTK_ENTRY(self->pre_shared_key_entry), FALSE);
    gtk_entry_set_input_purpose(GTK_ENTRY(self->pre_shared_key_entry), GTK_INPUT_PURPOSE_PASSWORD);

    attach_row(grid, 0, "Interface name", self->interface_name_entry);
    attach_row(grid, 1, "Hostname", self->hostname_entry);
    attach_row(grid, 2, "Pre-shared key", self->pre_shared_key_entry);

    gtk_box_pack_start(GTK_BOX(self->widget), section, FALSE, FALSE, 0);
}

static void
connect_ui_signals(NetbirdEditor *self)
{
    connect_changed(self, self->auth_combo);
    connect_changed(self, self->management_url_entry);
    connect_changed(self, self->admin_url_entry);
    connect_changed(self, self->profile_name_entry);
    connect_changed(self, self->username_entry);
    connect_changed(self, self->setup_key_entry);
    connect_changed(self, self->hint_entry);
    connect_changed(self, self->interface_name_entry);
    connect_changed(self, self->hostname_entry);
    connect_changed(self, self->pre_shared_key_entry);
}

static void
netbird_editor_init(NetbirdEditor *self)
{
    self->widget = gtk_box_new(GTK_ORIENTATION_VERTICAL, 12);
    gtk_container_set_border_width(GTK_CONTAINER(self->widget), 12);
    g_object_ref_sink(self->widget);

    build_main_section(self);
    build_setup_key_section(self);
    build_sso_section(self);
    build_advanced_section(self);
    connect_ui_signals(self);
}

static void
netbird_editor_dispose(GObject *object)
{
    NetbirdEditor *self = NETBIRD_EDITOR(object);

    g_clear_object(&self->widget);
    G_OBJECT_CLASS(netbird_editor_parent_class)->dispose(object);
}

static void
netbird_editor_class_init(NetbirdEditorClass *klass)
{
    GObjectClass *object_class = G_OBJECT_CLASS(klass);

    object_class->dispose = netbird_editor_dispose;
}

static GObject *
get_widget(NMVpnEditor *editor)
{
    return G_OBJECT(NETBIRD_EDITOR(editor)->widget);
}

static gboolean
update_connection(NMVpnEditor *editor, NMConnection *connection, GError **error)
{
    NetbirdEditor *self = NETBIRD_EDITOR(editor);
    NetbirdEditorValues values;
    gboolean ok;

    fill_values_from_ui(self, &values);
    ok = netbird_editor_values_save(&values, connection, error);
    netbird_editor_values_clear(&values);
    return ok;
}

static void
netbird_editor_iface_init(NMVpnEditorInterface *iface)
{
    iface->get_widget = get_widget;
    iface->update_connection = update_connection;
}

NMVpnEditor *
netbird_editor_new(NMConnection *connection, GError **error)
{
    NetbirdEditor *editor;
    NetbirdEditorValues values;

    (void) error;

    editor = g_object_new(NETBIRD_TYPE_EDITOR, NULL);

    netbird_editor_values_init(&values);
    netbird_editor_values_load(&values, connection);
    load_values_into_ui(editor, &values);
    netbird_editor_values_clear(&values);

    return NM_VPN_EDITOR(editor);
}
