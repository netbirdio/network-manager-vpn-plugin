MODULE := nm_netbird
SELINUX_MAKEFILE ?= /usr/share/selinux/devel/Makefile

.PHONY: all install clean

all: $(MODULE).pp

$(MODULE).pp: $(MODULE).te $(MODULE).fc
	@test -f "$(SELINUX_MAKEFILE)" || { \
		echo "error: missing $(SELINUX_MAKEFILE)" >&2; \
		echo "error: install selinux-policy-devel or set SELINUX_MAKEFILE=/path/to/Makefile" >&2; \
		exit 1; \
	}
	$(MAKE) -f "$(SELINUX_MAKEFILE)" $@

install: $(MODULE).pp
	semodule -i $<

clean:
	rm -rf tmp
	rm -f $(MODULE).pp $(MODULE).mod $(MODULE).mod.fc $(MODULE).if $(MODULE).fc.tmp
