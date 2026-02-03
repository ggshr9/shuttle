# OpenWrt Package Makefile
# Build with OpenWrt SDK:
#   1. Download SDK from https://downloads.openwrt.org/
#   2. Extract and cd into SDK directory
#   3. git clone https://github.com/shuttle-proxy/shuttle package/shuttle
#   4. make package/shuttle/compile V=s

include $(TOPDIR)/rules.mk

PKG_NAME:=shuttle
PKG_VERSION:=1.0.0
PKG_RELEASE:=1

PKG_SOURCE_PROTO:=git
PKG_SOURCE_URL:=https://github.com/shuttle-proxy/shuttle.git
PKG_SOURCE_VERSION:=main
PKG_MIRROR_HASH:=skip

PKG_LICENSE:=MIT
PKG_MAINTAINER:=Shuttle Proxy <shuttle@example.com>

PKG_BUILD_DEPENDS:=golang/host
PKG_BUILD_PARALLEL:=1
PKG_BUILD_FLAGS:=no-mips16

GO_PKG:=github.com/shuttle-proxy/shuttle
GO_PKG_BUILD_PKG:=github.com/shuttle-proxy/shuttle/cmd/shuttle
GO_PKG_LDFLAGS_X:=main.version=$(PKG_VERSION)

include $(INCLUDE_DIR)/package.mk
include ../../lang/golang/golang-package.mk

define Package/shuttle
  SECTION:=net
  CATEGORY:=Network
  SUBMENU:=Web Servers/Proxies
  TITLE:=Multi-transport proxy client
  URL:=https://github.com/shuttle-proxy/shuttle
  DEPENDS:=$(GO_ARCH_DEPENDS) +kmod-tun +ip-full
endef

define Package/shuttle/description
  Shuttle is a multi-transport proxy system for high-censorship environments.
  Features H3/QUIC, Reality/TLS+Noise, and CDN transports with adaptive
  congestion control and intelligent routing.
endef

define Package/shuttle/conffiles
/etc/shuttle/client.yaml
endef

define Package/shuttle/install
	$(call GoPackage/Package/Install/Bin,$(PKG_INSTALL_DIR))
	$(INSTALL_DIR) $(1)/usr/bin
	$(INSTALL_BIN) $(PKG_INSTALL_DIR)/usr/bin/shuttle $(1)/usr/bin/

	$(INSTALL_DIR) $(1)/etc/shuttle
	$(INSTALL_CONF) ./files/client.yaml $(1)/etc/shuttle/

	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) ./files/shuttle.init $(1)/etc/init.d/shuttle
endef

# Server package
define Package/shuttled
  SECTION:=net
  CATEGORY:=Network
  SUBMENU:=Web Servers/Proxies
  TITLE:=Multi-transport proxy server
  URL:=https://github.com/shuttle-proxy/shuttle
  DEPENDS:=$(GO_ARCH_DEPENDS)
endef

define Package/shuttled/description
  Shuttle server daemon for the multi-transport proxy system.
endef

GO_PKG_BUILD_PKG_shuttled:=github.com/shuttle-proxy/shuttle/cmd/shuttled

define Package/shuttled/install
	$(call GoPackage/Package/Install/Bin,$(PKG_INSTALL_DIR))
	$(INSTALL_DIR) $(1)/usr/bin
	$(INSTALL_BIN) $(PKG_INSTALL_DIR)/usr/bin/shuttled $(1)/usr/bin/

	$(INSTALL_DIR) $(1)/etc/shuttle
	$(INSTALL_CONF) ./files/server.yaml $(1)/etc/shuttle/

	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) ./files/shuttled.init $(1)/etc/init.d/shuttled
endef

$(eval $(call GoBinPackage,shuttle))
$(eval $(call BuildPackage,shuttle))
$(eval $(call GoBinPackage,shuttled))
$(eval $(call BuildPackage,shuttled))
