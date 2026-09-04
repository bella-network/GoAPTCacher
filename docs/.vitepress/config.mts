import { defineConfig } from "vitepress";

export default defineConfig({
  lang: "en-US",
  title: "GoAPTCacher",
  titleTemplate: ":title | GoAPTCacher",
  description: "Pull-through caching proxy for Debian and Ubuntu APT repositories",

  cleanUrls: true,
  lastUpdated: true,
  sitemap: {
    hostname: "https://goaptcacher.docs.bella.network"
  },
  markdown: {
    lineNumbers: true
  },

  head: [["meta", { name: "theme-color", content: "#0f766e" }]],

  themeConfig: {
    siteTitle: "GoAPTCacher",
    outline: [2, 3],

    search: {
      provider: "local"
    },

    nav: [
      { text: "Guide", link: "/guide/getting-started" },
      { text: "Concepts", link: "/concepts/architecture" },
      { text: "Features", link: "/features/domain-routing" },
      { text: "Operations", link: "/operations/web-interface" },
      { text: "Examples", link: "/examples/home-lab" },
      { text: "Security", link: "/security/best-practices" },
      { text: "Reference", link: "/reference/configuration" }
    ],

    sidebar: [
      {
        text: "Guide",
        items: [
          { text: "Getting started", link: "/guide/getting-started" },
          { text: "Installation", link: "/guide/installation" },
          { text: "Container deployment", link: "/guide/container" },
          { text: "Client configuration", link: "/guide/client-configuration" },
          { text: "Configuration", link: "/guide/configuration" }
        ]
      },
      {
        text: "Concepts",
        items: [
          { text: "Architecture", link: "/concepts/architecture" },
          { text: "Cache lifecycle", link: "/concepts/cache-lifecycle" },
          { text: "HTTPS modes", link: "/concepts/https-modes" }
        ]
      },
      {
        text: "Features",
        items: [
          { text: "Domain and mirror routing", link: "/features/domain-routing" },
          { text: "Automatic discovery", link: "/features/discovery" }
        ]
      },
      {
        text: "Operations",
        items: [
          { text: "Web interface and API", link: "/operations/web-interface" },
          { text: "Cache management", link: "/operations/cache-management" },
          { text: "Repository verification", link: "/operations/repository-verification" },
          { text: "Logging and debugging", link: "/operations/logging-debugging" },
          { text: "Troubleshooting", link: "/operations/troubleshooting" }
        ]
      },
      {
        text: "Examples",
        items: [
          { text: "Home lab", link: "/examples/home-lab" },
          { text: "CI runners", link: "/examples/ci-runners" },
          { text: "Restricted network", link: "/examples/restricted-network" }
        ]
      },
      {
        text: "Security",
        items: [
          { text: "Best practices", link: "/security/best-practices" }
        ]
      },
      {
        text: "Reference",
        items: [
          { text: "Configuration", link: "/reference/configuration" },
          { text: "CLI and environment", link: "/reference/cli-environment" },
          { text: "HTTP endpoints", link: "/reference/endpoints" },
          { text: "Response headers", link: "/reference/response-headers" }
        ]
      }
    ],

    socialLinks: [
      {
        icon: "gitlab",
        link: "https://gitlab.com/bella.network/goaptcacher"
      }
    ],

    editLink: {
      pattern: "https://gitlab.com/bella.network/goaptcacher/-/edit/main/docs/:path",
      text: "Edit this page on GitLab"
    },

    footer: {
      message: "Released under the MIT License.",
      copyright: "Copyright © 2026 Thomas Bella"
    }
  }
});
