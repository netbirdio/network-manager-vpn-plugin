package nmplugin

import "github.com/godbus/dbus/v5"

type properties struct {
	service *Service
}

func (p *properties) Get(interfaceName string, propertyName string) (dbus.Variant, *dbus.Error) {
	if interfaceName != Interface {
		return dbus.Variant{}, newDBusError(dbusErrorUnknownInterface, "unknown interface %q", interfaceName)
	}

	switch propertyName {
	case "State":
		return dbus.MakeVariant(uint32(p.service.State())), nil
	default:
		return dbus.Variant{}, newDBusError(dbusErrorUnknownProperty, "unknown property %q", propertyName)
	}
}

func (p *properties) GetAll(interfaceName string) (map[string]dbus.Variant, *dbus.Error) {
	if interfaceName != Interface {
		return nil, newDBusError(dbusErrorUnknownInterface, "unknown interface %q", interfaceName)
	}

	return map[string]dbus.Variant{
		"State": dbus.MakeVariant(uint32(p.service.State())),
	}, nil
}

func (p *properties) Set(interfaceName string, propertyName string, value dbus.Variant) *dbus.Error {
	if interfaceName != Interface {
		return newDBusError(dbusErrorUnknownInterface, "unknown interface %q", interfaceName)
	}
	if propertyName != "State" {
		return newDBusError(dbusErrorUnknownProperty, "unknown property %q", propertyName)
	}
	return newDBusError(dbusErrorPropertyReadOnly, "property %q is read-only", propertyName)
}
