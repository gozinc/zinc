// Builds llms.txt and llms-full.txt (https://llmstxt.org) from the docs, in
// sidebar order, so an AI assistant can read what Zinc actually offers.
import { getCollection, type CollectionEntry } from "astro:content";
import { sidebar } from "../sidebar.mjs";

type Item = { label: string; link?: string; items?: Item[] };
type Section = { title: string; pages: CollectionEntry<"docs">[] };

const site = "https://zinc.carbonsoft.sh";

export const summary = `# Zinc

> Zinc is a Go web framework on net/http, with an Express-style API, typed handlers and an OpenAPI 3.1 spec generated from your Go types. The core has no dependencies.

- Import path: \`github.com/0mjs/zinc\`. It needs Go 1.25 or newer.
- A handler is \`func(c *zinc.Context) error\`: return an error, or write a response with \`c.JSON\`, \`c.String\` and the like.
- Path parameters are written \`{id}\`, and a rest-of-path match \`{path...}\`, as in net/http. Gin and Echo's \`:id\` and \`*path\` syntax is rejected at startup.
- \`zinc.Typed(func(c *zinc.Context, in In) (Out, error))\` binds and validates \`In\` from the request and documents both types in the spec.
- Struct tags choose where a field comes from: \`path\`, \`query\`, \`header\`, \`cookie\`, \`form\`, \`json\`, \`xml\`; \`validate\`, \`default\`, \`enum\` and \`pattern\` add rules.
- Every app serves \`/openapi.json\` and \`/docs\`. An \`App\` is an \`http.Handler\`.
- Middleware lives in \`github.com/0mjs/zinc/middleware/<name>\`, one package each, such as \`cors\`, \`logger\` and \`limiter\`.`;

function flatten(items: Item[], out: Item[] = []): Item[] {
  for (const item of items) {
    if (item.link) out.push(item);
    if (item.items) flatten(item.items, out);
  }
  return out;
}

export async function sections(): Promise<Section[]> {
  const entries = await getCollection("docs");
  const byLink = new Map(entries.map((e) => [`/${e.id.replace(/\/index$/, "")}/`.replace("//", "/"), e]));
  return (sidebar as Item[]).map((group) => ({
    title: group.label,
    pages: flatten(group.items ?? [])
      .map((item) => byLink.get(item.link!))
      .filter((e): e is CollectionEntry<"docs"> => e !== undefined),
  }));
}

export const url = (e: CollectionEntry<"docs">) => `${site}/${e.id}/`;
