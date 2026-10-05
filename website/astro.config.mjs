import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";
import { ExpressiveCodeTheme } from "@astrojs/starlight/expressive-code";
import { readFileSync } from "node:fs";
import { sidebar } from "./src/sidebar.mjs";

const zincFrost = ExpressiveCodeTheme.fromJSONString(
  readFileSync(new URL("./src/themes/zinc-frost.json", import.meta.url), "utf-8"),
);

// Code frames are dark plates in both site themes.
const plate = {
  bg: "#0f1215",
  bar: "#13161a",
  line: "#1f242a",
  mute: "#7d889a",
};

export default defineConfig({
  site: "https://zinc.carbonsoft.sh",
  redirects: {
    "/getting-started/installation": "/guide/installation",
    "/getting-started/quick-start": "/guide/quickstart",
    "/getting-started/first-route": "/guide/first-route",
    "/guide/error-handling": "/guide/errors",
    "/guide/response": "/guide/responses-and-rendering",
    "/cookbook/hello-world": "/guide/quickstart",
    "/cookbook/cors": "/middleware/cors",
    "/cookbook/http2-server-push": "/cookbook/http2",
    "/cookbook/jsonp": "/guide/responses-and-rendering",
    "/cookbook/load-balancing": "/middleware/proxy",
    "/cookbook/subdomain": "/guide/routing",
    "/middleware/request-id": "/middleware/requestid",
    "/middleware/request-logger": "/middleware/logger",
    "/middleware/body-limit": "/middleware/bodylimit",
    "/middleware/context-timeout": "/middleware/timeout",
    "/middleware/rate-limiter": "/middleware/limiter",
    "/middleware/basic-auth": "/middleware/basicauth",
    "/middleware/key-auth": "/middleware/keyauth",
    "/middleware/casbin-auth": "/middleware/casbin",
    "/middleware/header-guards": "/middleware/contenttype",
    "/middleware/static": "/guide/static-files",
    "/middleware/jaeger": "/middleware/open-telemetry",
    "/middleware/body-dump": "/middleware/bodydump",
    "/middleware/method-override": "/middleware/methodoverride",
    "/middleware/trailing-slash": "/middleware/trailingslash",
    "/middleware/utility": "/middleware/overview",
    "/middleware/jwt": "/middleware/jwtauth",
    "/middleware/gzip": "/middleware/compress",
  },
  integrations: [
    starlight({
      title: "Zinc",
      favicon: "/favicon.png?v=20260923",
      customCss: ["./src/styles/zinc.css"],
      editLink: {
        baseUrl: "https://github.com/gozinc/zinc/edit/dev/website/",
      },
      lastUpdated: true,
      social: [
        {
          icon: "github",
          label: "GitHub",
          href: "https://github.com/gozinc/zinc",
        },
      ],
      components: {
        Footer: "./src/components/Footer.astro",
        SiteTitle: "./src/components/SiteTitle.astro",
        PageTitle: "./src/components/PageTitle.astro",
      },
      head: [
        {
          tag: "script",
          content:
            "try{if(!localStorage.getItem('starlight-theme')){localStorage.setItem('starlight-theme','dark');document.documentElement.dataset.theme='dark';}}catch(e){document.documentElement.dataset.theme='dark';}",
        },
        {
          tag: "link",
          attrs: { rel: "preconnect", href: "https://fonts.googleapis.com" },
        },
        {
          tag: "link",
          attrs: { rel: "apple-touch-icon", href: "/apple-touch-icon.png" },
        },
        {
          tag: "link",
          attrs: {
            rel: "preconnect",
            href: "https://fonts.gstatic.com",
            crossorigin: true,
          },
        },
        {
          tag: "link",
          attrs: {
            rel: "stylesheet",
            href: "https://fonts.googleapis.com/css2?family=Michroma&family=Instrument+Sans:wght@400;500;600;700&family=IBM+Plex+Mono:ital,wght@0,400;0,500;0,600;1,400;1,500&display=swap",
          },
        },
      ],
      sidebar,
      expressiveCode: {
        themes: [zincFrost],
        useStarlightDarkModeSwitch: false,
        useStarlightUiThemeColors: false,
        styleOverrides: {
          borderRadius: "6px",
          borderColor: plate.line,
          codeFontFamily: "var(--sl-font-mono)",
          codeFontSize: "0.8rem",
          codeFontWeight: "500",
          codeLineHeight: "1.7",
          codePaddingBlock: "1rem",
          codePaddingInline: "1.15rem",
          uiFontFamily: "var(--sl-font-mono)",
          scrollbarThumbColor: "#2c323a",
          scrollbarThumbHoverColor: "#3a414b",
          frames: {
            frameBoxShadowCssValue: "none",
            editorBackground: plate.bg,
            editorTabBarBackground: plate.bar,
            editorTabBarBorderColor: "transparent",
            editorTabBarBorderBottomColor: plate.line,
            editorActiveTabBackground: plate.bar,
            editorActiveTabForeground: "#aeb8c6",
            editorActiveTabBorderColor: "transparent",
            editorActiveTabIndicatorTopColor: "transparent",
            editorActiveTabIndicatorBottomColor: "transparent",
            editorTabBorderRadius: "0",
            terminalBackground: plate.bg,
            terminalTitlebarBackground: plate.bar,
            terminalTitlebarForeground: plate.mute,
            terminalTitlebarBorderBottomColor: plate.line,
            terminalTitlebarDotsForeground: "#aeb8c6",
            terminalTitlebarDotsOpacity: "1",
            inlineButtonBackground: "#d8dee9",
            inlineButtonForeground: "#aeb8c6",
            inlineButtonBorder: plate.line,
            inlineButtonBorderOpacity: "1",
            tooltipSuccessBackground: "#d8dee9",
            tooltipSuccessForeground: plate.bg,
          },
        },
      },
    }),
  ],
});
