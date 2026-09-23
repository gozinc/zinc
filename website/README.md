# Zinc documentation

The Zinc documentation site is built with Astro and Starlight.

```sh
npm install
npm run dev
```

The local site runs at <http://localhost:4321>. Documentation lives in
`src/content/docs`, the homepage lives in `src/components/HomeHero.astro`, and
the sidebar is configured in `astro.config.mjs`.

The theme lives in `src/styles/zinc.css`, and code blocks use the Nord-derived
`src/themes/zinc-frost.json` theme. The spangle texture and favicons in `public/`
are generated; rebuild them with `npm run build:brand`.

Before publishing:

```sh
npm run check:site
npm run check:examples
npm run check:templ
```

## Cloudflare Workers

Cloudflare Workers Builds is connected to `0mjs/zinc`. Every push or merged
pull request to `dev` automatically builds and deploys the docs to
<https://zinc.carbonsoft.sh>. Other branches do not deploy to production.

The `zinc` Worker's build settings are:

- Production branch: `dev`
- Root directory: `website`
- Build command: `npm run check:site`
- Deploy command: `npx wrangler deploy`
- Build cache: enabled

The build must pass the source-doc, Astro build, and generated-link checks
before deployment. Cloudflare stores the deployment credential; GitHub Actions
and repository secrets are not required. Manage this connection in the
Cloudflare dashboard under **zinc → Settings → Builds**.

The documentation is deployed with Workers Static Assets from `dist`:

```sh
npm run deploy:dry-run
npm run deploy
```

Run `npm run check:site` before publishing; it validates the source docs, builds
all routes, and checks every generated internal link.
