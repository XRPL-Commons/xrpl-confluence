import { defineConfig } from 'vitepress';
import llmstxt from 'vitepress-plugin-llms';
import { copyOrDownloadAsMarkdownButtons } from 'vitepress-plugin-llms';

export default defineConfig({
  title: 'XRPL Confluence',
  description:
    'Kurtosis harness orchestrating mixed rippled + go-xrpl networks for propagation, sync, consensus, soak, chaos, fuzz and replay testing.',

  lang: 'en-US',
  base: '/xrpl-confluence/',

  head: [
    ['link', { rel: 'icon', type: 'image/png', href: '/xrpl-confluence/favicon.png' }],
    ['link', { rel: 'preconnect', href: 'https://fonts.googleapis.com' }],
    ['link', { rel: 'preconnect', href: 'https://fonts.gstatic.com', crossorigin: '' }],
    [
      'link',
      {
        href: 'https://fonts.googleapis.com/css2?family=Unbounded:wght@400;500;600;700;800&family=Plus+Jakarta+Sans:wght@400;500;600;700&display=swap',
        rel: 'stylesheet',
      },
    ],
  ],

  themeConfig: {
    logo: '/commons_ligth_logo.png',

    nav: [
      {
        text: 'Guide',
        items: [
          { text: 'Overview', link: '/overview' },
          { text: 'Quickstart', link: '/quickstart' },
        ],
      },
      {
        text: 'Reference',
        items: [
          { text: 'Test Suites', link: '/test-suites' },
          { text: 'Topology & Control', link: '/topology' },
          { text: 'Sidecar & Oracle', link: '/sidecar-oracle' },
          { text: 'Dashboard', link: '/dashboard' },
          { text: 'Chaos', link: '/chaos' },
          { text: 'CLI & Scenarios', link: '/cli' },
        ],
      },
      {
        text: 'Links',
        items: [
          { text: 'GitHub', link: 'https://github.com/XRPL-Commons/xrpl-confluence' },
          { text: 'XRPL Commons', link: 'https://www.xrpl-commons.org' },
        ],
      },
    ],

    sidebar: [
      {
        text: 'Getting Started',
        items: [
          { text: 'Overview', link: '/overview' },
          { text: 'Quickstart', link: '/quickstart' },
        ],
      },
      {
        text: 'Testing',
        items: [
          { text: 'Test Suites', link: '/test-suites' },
          { text: 'Chaos', link: '/chaos' },
        ],
      },
      {
        text: 'Architecture',
        items: [
          { text: 'Topology & Control', link: '/topology' },
          { text: 'Sidecar & Oracle', link: '/sidecar-oracle' },
          { text: 'Dashboard', link: '/dashboard' },
        ],
      },
      {
        text: 'Reference',
        items: [{ text: 'CLI & Scenarios', link: '/cli' }],
      },
    ],

    socialLinks: [{ icon: 'github', link: 'https://github.com/XRPL-Commons/xrpl-confluence' }],

    footer: {
      message: 'Released under the MIT License.',
      copyright: 'Copyright © 2026 XRPL Commons',
    },

    search: {
      provider: 'local',
    },
  },

  markdown: {
    lineNumbers: true,
    config(md) {
      md.use(copyOrDownloadAsMarkdownButtons);
    },
  },

  vite: {
    plugins: [
      llmstxt({
        generateLLMsFullTxt: true,
        ignoreFiles: [],
      }),
    ],
  },

  srcExclude: ['**/README.md', 'assets/**'],
});
