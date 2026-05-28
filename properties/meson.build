libnm_dep = dependency('libnm', version: '>= 1.20')
gtk_dep = dependency('gtk+-3.0', version: '>= 3.22')

netbird_editor_sources = files(
  'nm-netbird-editor-model.c',
  'nm-netbird-editor.c',
  'nm-netbird-editor-plugin.c',
)

netbird_plugin = shared_module(
  'nm-vpn-plugin-netbird',
  netbird_editor_sources,
  dependencies: [libnm_dep, gtk_dep],
  name_prefix: 'lib',
  install: true,
  install_dir: get_option('nm_plugin_dir'),
)

netbird_model_test = executable(
  'nm-netbird-editor-model-test',
  files('nm-netbird-editor-model.c', 'nm-netbird-editor-model-test.c'),
  dependencies: [libnm_dep],
)

test('netbird-editor-model', netbird_model_test)

netbird_plugin_test = executable(
  'nm-netbird-editor-plugin-test',
  files('nm-netbird-editor-plugin-test.c'),
  dependencies: [libnm_dep],
)

test(
  'netbird-editor-plugin-load',
  netbird_plugin_test,
  env: ['NETBIRD_EDITOR_PLUGIN_PATH=' + netbird_plugin.full_path()],
)
