libnm_dep = dependency('libnm', version: '>= 1.20')
gtk_dep = dependency('gtk+-3.0', version: '>= 3.22')
libnma_dep = dependency('libnma', version: '>= 1.8.0')
gtk3_c_args = [
  '-DGDK_VERSION_MIN_REQUIRED=GDK_VERSION_3_22',
  '-DGDK_VERSION_MAX_ALLOWED=GDK_VERSION_3_22',
]
cc = meson.get_compiler('c')
dl_dep = cc.find_library('dl', required: false)

netbird_editor_sources = files(
  'nm-netbird-editor-model.c',
  'nm-netbird-editor.c',
)

netbird_plugin_deps = [libnm_dep]
if dl_dep.found()
  netbird_plugin_deps += dl_dep
endif

netbird_plugin = shared_module(
  'nm-vpn-plugin-netbird',
  files('nm-netbird-editor-plugin.c'),
  dependencies: netbird_plugin_deps,
  name_prefix: 'lib',
  install: true,
  install_dir: get_option('nm_plugin_dir'),
)

shared_module(
  'nm-vpn-plugin-netbird-editor',
  netbird_editor_sources,
  c_args: gtk3_c_args,
  dependencies: [libnm_dep, gtk_dep, libnma_dep],
  name_prefix: 'lib',
  install: true,
  install_dir: get_option('nm_plugin_dir'),
)

if get_option('gtk4')
  gtk4_dep = dependency('gtk4', version: '>= 4.0')
  libnma_gtk4_dep = dependency('libnma-gtk4', version: '>= 1.8.33')
  gtk4_c_args = [
    '-DGDK_VERSION_MIN_REQUIRED=GDK_VERSION_4_0',
    '-DGDK_VERSION_MAX_ALLOWED=GDK_VERSION_4_0',
  ]

  shared_module(
    'nm-gtk4-vpn-plugin-netbird-editor',
    netbird_editor_sources,
    c_args: gtk4_c_args,
    dependencies: [libnm_dep, gtk4_dep, libnma_gtk4_dep],
    name_prefix: 'lib',
    install: true,
    install_dir: get_option('nm_plugin_dir'),
  )
endif

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
