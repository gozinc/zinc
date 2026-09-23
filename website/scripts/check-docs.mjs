import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const docsRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../src/content/docs");
const repoRoot = path.resolve(docsRoot, "../../../../");
const failures = [];

function filesBelow(root, accept) {
  const files = [];
  for (const entry of fs.readdirSync(root, { withFileTypes: true })) {
    const filename = path.join(root, entry.name);
    if (entry.isDirectory()) files.push(...filesBelow(filename, accept));
    else if (accept(filename)) files.push(filename);
  }
  return files;
}

const docs = filesBelow(docsRoot, (filename) => /\.mdx?$/.test(filename));
const rootGoFiles = fs
  .readdirSync(repoRoot)
  .filter((filename) => filename.endsWith(".go"))
  .map((filename) => path.join(repoRoot, filename));
const middlewareGoFiles = filesBelow(path.join(repoRoot, "middleware"), (filename) => filename.endsWith(".go"));

function routeFor(filename, source) {
  const explicit = source.match(/^---[\s\S]*?^slug:\s*(\S+)[\s\S]*?^---/m)?.[1];
  if (explicit) return normalizeRoute(explicit.startsWith("/") ? explicit : `/${explicit}`);

  const relative = path.relative(docsRoot, filename).replace(/\\/g, "/").replace(/\.mdx?$/, "");
  return normalizeRoute(`/${relative.replace(/(^|\/)index$/, "")}`);
}

function normalizeRoute(route) {
  const withoutFile = route.replace(/\.mdx?$/, "");
  const normalized = path.posix.normalize(withoutFile).replace(/\/$/, "");
  return normalized === "." || normalized === "" ? "/" : normalized;
}

const sources = new Map(docs.map((filename) => [filename, fs.readFileSync(filename, "utf8")]));
const routes = new Set([...sources].map(([filename, source]) => routeFor(filename, source)));

const astroConfig = fs.readFileSync(path.join(repoRoot, "website/astro.config.mjs"), "utf8");
for (const match of astroConfig.matchAll(/^\s*"(\/[^"{]+)":\s*"\//gm)) {
  routes.add(normalizeRoute(match[1]));
}

function checkLinks(filename, source) {
  const prose = source.replace(/```[\s\S]*?```/g, "").replace(/`[^`\n]+`/g, "");
  const hrefs = [
    ...[...prose.matchAll(/\[[^\]]+\]\(([^)]+)\)/g)].map((match) => match[1]),
    ...[...prose.matchAll(/\bhref=["']([^"']+)["']/g)].map((match) => match[1]),
  ];

  for (const rawHref of hrefs) {
    const href = rawHref.trim().split(/\s+["']/)[0];
    if (!href || /^(?:https?:|mailto:|#)/.test(href)) continue;
    if (/\.[a-z0-9]+(?:[?#].*)?$/i.test(href) && !/\.mdx?(?:[?#].*)?$/i.test(href)) continue;

    const cleanHref = href.split(/[?#]/)[0];
    const route = cleanHref.startsWith("/")
      ? normalizeRoute(cleanHref)
      : normalizeRoute(path.posix.resolve(`${routeFor(filename, source)}/`, cleanHref));
    if (!routes.has(route)) {
      failures.push(`${path.relative(repoRoot, filename)}: unresolved link ${rawHref}`);
    }
  }
}

function topLevelExports(files) {
  const names = new Set();
  for (const filename of files) {
    const source = fs.readFileSync(filename, "utf8");
    for (const match of source.matchAll(/^(?:func|type|var|const)\s+(?:\([^\n]+\)\s+)?([A-Z][A-Za-z0-9_]*)/gm)) {
      names.add(match[1]);
    }
    for (const block of source.matchAll(/(?:const|var)\s*\(([\s\S]*?)\n\)/g)) {
      for (const match of block[1].matchAll(/^\s*([A-Z][A-Za-z0-9_]*)\b/gm)) names.add(match[1]);
    }
  }
  return names;
}

function methodsFor(typeName) {
  const names = new Set();
  const pattern = new RegExp(`^func\\s+\\([^\\n]*\\*?${typeName}\\)\\s+([A-Z][A-Za-z0-9_]*)`, "gm");
  for (const filename of rootGoFiles) {
    const source = fs.readFileSync(filename, "utf8");
    for (const match of source.matchAll(pattern)) names.add(match[1]);
  }
  return names;
}

const zincExports = topLevelExports(rootGoFiles);
const middlewareExports = topLevelExports(middlewareGoFiles);
const methodSets = {
  app: methodsFor("App"),
  c: methodsFor("Context"),
  group: methodsFor("Group"),
};
const removedAPIMentions = new Set(["zinc.ParamIdentifier", "zinc.WildcardIdentifier", "c.Copy"]);

function checkAPI(filename, source) {
  const relative = path.relative(repoRoot, filename);
  for (const match of source.matchAll(/\bzinc\.([A-Z][A-Za-z0-9_]*)/g)) {
    const reference = `zinc.${match[1]}`;
    if (!zincExports.has(match[1]) && !(relative.endsWith("migration-0.2.md") && removedAPIMentions.has(reference))) {
      failures.push(`${relative}: unknown API ${reference}`);
    }
  }
  for (const match of source.matchAll(/\bmiddleware\.([A-Z][A-Za-z0-9_]*)/g)) {
    if (!middlewareExports.has(match[1])) failures.push(`${relative}: unknown API middleware.${match[1]}`);
  }
  for (const [receiver, methods] of Object.entries(methodSets)) {
    for (const match of source.matchAll(new RegExp(`\\b${receiver}\\.([A-Z][A-Za-z0-9_]*)`, "g"))) {
      const reference = `${receiver}.${match[1]}`;
      if (!methods.has(match[1]) && !(relative.endsWith("migration-0.2.md") && removedAPIMentions.has(reference))) {
        failures.push(`${relative}: unknown API ${reference}`);
      }
    }
  }
}

for (const [filename, source] of sources) {
  checkLinks(filename, source);
  checkAPI(filename, source);
}

if (failures.length > 0) {
  console.error(failures.join("\n"));
  process.exit(1);
}

console.log(`Checked ${docs.length} documentation pages: internal links and referenced Zinc APIs are valid.`);
