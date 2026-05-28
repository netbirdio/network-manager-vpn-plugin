package nmplugin

// IntrospectionXML describes the minimal NetworkManager VPN plugin surface this
// process exports. It intentionally matches the interface installed by
// NetworkManager at /usr/share/dbus-1/interfaces/org.freedesktop.NetworkManager.VPN.Plugin.xml.
const IntrospectionXML = `<?xml version="1.0" encoding="UTF-8"?>
<node>
  <interface name="org.freedesktop.DBus.Introspectable">
    <method name="Introspect">
      <arg name="xml_data" type="s" direction="out"/>
    </method>
  </interface>

  <interface name="org.freedesktop.DBus.Properties">
    <method name="Get">
      <arg name="interface_name" type="s" direction="in"/>
      <arg name="property_name" type="s" direction="in"/>
      <arg name="value" type="v" direction="out"/>
    </method>
    <method name="GetAll">
      <arg name="interface_name" type="s" direction="in"/>
      <arg name="props" type="a{sv}" direction="out"/>
    </method>
    <method name="Set">
      <arg name="interface_name" type="s" direction="in"/>
      <arg name="property_name" type="s" direction="in"/>
      <arg name="value" type="v" direction="in"/>
    </method>
    <signal name="PropertiesChanged">
      <arg name="interface_name" type="s"/>
      <arg name="changed_properties" type="a{sv}"/>
      <arg name="invalidated_properties" type="as"/>
    </signal>
  </interface>

  <interface name="org.freedesktop.NetworkManager.VPN.Plugin">
    <method name="Connect">
      <arg name="connection" type="a{sa{sv}}" direction="in"/>
    </method>

    <method name="ConnectInteractive">
      <arg name="connection" type="a{sa{sv}}" direction="in"/>
      <arg name="details" type="a{sv}" direction="in"/>
    </method>

    <method name="NeedSecrets">
      <arg name="settings" type="a{sa{sv}}" direction="in"/>
      <arg name="setting_name" type="s" direction="out"/>
    </method>

    <method name="Disconnect"/>

    <method name="SetConfig">
      <arg name="config" type="a{sv}" direction="in"/>
    </method>

    <method name="SetIp4Config">
      <arg name="config" type="a{sv}" direction="in"/>
    </method>

    <method name="SetIp6Config">
      <arg name="config" type="a{sv}" direction="in"/>
    </method>

    <method name="SetFailure">
      <arg name="reason" type="s" direction="in"/>
    </method>

    <property name="State" type="u" access="read"/>

    <signal name="StateChanged">
      <arg name="state" type="u"/>
    </signal>

    <signal name="SecretsRequired">
      <arg name="message" type="s"/>
      <arg name="secrets" type="as"/>
    </signal>

    <method name="NewSecrets">
      <arg name="connection" type="a{sa{sv}}" direction="in"/>
    </method>

    <signal name="Config">
      <arg name="config" type="a{sv}"/>
    </signal>

    <signal name="Ip4Config">
      <arg name="ip4config" type="a{sv}"/>
    </signal>

    <signal name="Ip6Config">
      <arg name="ip6config" type="a{sv}"/>
    </signal>

    <signal name="LoginBanner">
      <arg name="banner" type="s"/>
    </signal>

    <signal name="Failure">
      <arg name="reason" type="u"/>
    </signal>
  </interface>
</node>`
