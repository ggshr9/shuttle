import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'Shuttle',
  description: 'Multi-Transport Network Toolkit',

  locales: {
    en: {
      label: 'English',
      lang: 'en',
      link: '/en/',
      themeConfig: {
        nav: [
          {
            text: 'Guide',
            items: [
              { text: 'Getting Started', link: '/en/guide/getting-started' },
              { text: 'Configuration Reference', link: '/en/guide/configuration' },
              { text: 'Migrate from Clash', link: '/en/guide/migrate-from-clash' },
            ],
          },
          {
            text: 'Protocols',
            items: [
              {
                text: 'Shuttle Native',
                items: [
                  { text: 'H3', link: '/en/protocols/native/h3' },
                  { text: 'Reality', link: '/en/protocols/native/reality' },
                  { text: 'CDN', link: '/en/protocols/native/cdn' },
                ],
              },
              {
                text: 'Compatible',
                items: [
                  { text: 'Shadowsocks', link: '/en/protocols/compat/shadowsocks' },
                  { text: 'VLESS', link: '/en/protocols/compat/vless' },
                  { text: 'Trojan', link: '/en/protocols/compat/trojan' },
                  { text: 'Hysteria2', link: '/en/protocols/compat/hysteria2' },
                  { text: 'VMess', link: '/en/protocols/compat/vmess' },
                  { text: 'WireGuard', link: '/en/protocols/compat/wireguard' },
                ],
              },
            ],
          },
          {
            text: 'Features',
            items: [
              { text: 'Proxy Groups', link: '/en/features/proxy-groups' },
              { text: 'Providers', link: '/en/features/providers' },
              { text: 'fake-ip DNS', link: '/en/features/fake-ip-dns' },
              { text: 'Mesh VPN', link: '/en/features/mesh-vpn' },
              { text: 'Congestion Control', link: '/en/features/congestion-control' },
              { text: 'Multipath', link: '/en/features/multipath' },
            ],
          },
          {
            text: 'API',
            items: [
              { text: 'REST API', link: '/en/api/rest' },
            ],
          },
        ],
      },
    },
    zh: {
      label: '中文',
      lang: 'zh-CN',
      link: '/zh/',
      themeConfig: {
        nav: [
          {
            text: '指南',
            items: [
              { text: '快速开始', link: '/zh/guide/getting-started' },
              { text: '配置参考', link: '/zh/guide/configuration' },
              { text: '从 Clash 迁移', link: '/zh/guide/migrate-from-clash' },
            ],
          },
          {
            text: '协议',
            items: [
              {
                text: 'Shuttle 原生',
                items: [
                  { text: 'H3', link: '/zh/protocols/native/h3' },
                  { text: 'Reality', link: '/zh/protocols/native/reality' },
                  { text: 'CDN', link: '/zh/protocols/native/cdn' },
                ],
              },
              {
                text: '兼容协议',
                items: [
                  { text: 'Shadowsocks', link: '/zh/protocols/compat/shadowsocks' },
                  { text: 'VLESS', link: '/zh/protocols/compat/vless' },
                  { text: 'Trojan', link: '/zh/protocols/compat/trojan' },
                  { text: 'Hysteria2', link: '/zh/protocols/compat/hysteria2' },
                  { text: 'VMess', link: '/zh/protocols/compat/vmess' },
                  { text: 'WireGuard', link: '/zh/protocols/compat/wireguard' },
                ],
              },
            ],
          },
          {
            text: '功能',
            items: [
              { text: '策略组', link: '/zh/features/proxy-groups' },
              { text: 'Provider', link: '/zh/features/providers' },
              { text: 'fake-ip DNS', link: '/zh/features/fake-ip-dns' },
              { text: 'Mesh VPN', link: '/zh/features/mesh-vpn' },
              { text: '拥塞控制', link: '/zh/features/congestion-control' },
              { text: '多路径', link: '/zh/features/multipath' },
            ],
          },
          {
            text: 'API',
            items: [
              { text: 'REST API', link: '/zh/api/rest' },
            ],
          },
        ],
      },
    },
  },

  themeConfig: {
    socialLinks: [
      { icon: 'github', link: 'https://github.com/your-org/shuttle' },
    ],
  },
})
