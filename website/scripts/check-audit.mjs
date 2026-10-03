// Runs npm audit and fails on any advisory, at any severity, except those
// listed below. Each exception says why it can't affect this site and when to
// look at it again, so it can't quietly outlive its reason.
import { execFileSync } from "node:child_process";

const exceptions = {
  // http-cache-semantics: max-stale handling in an HTTP cache can serve one
  // user's cached response to another. Astro depends on it, and no version is
  // patched. The docs are a static site with no server-side response cache,
  // so the code never serves a request here.
  "GHSA-ch52-4w7c-c8xp": { review: "2026-11-01" },
};

let report;
try {
  report = execFileSync("npm", ["audit", "--json"], { encoding: "utf8", stdio: ["ignore", "pipe", "inherit"] });
} catch (err) {
  report = err.stdout; // npm audit exits non-zero when it finds anything
}
const { vulnerabilities = {} } = JSON.parse(report);

const found = new Map(); // advisory id -> package and severity
for (const [name, v] of Object.entries(vulnerabilities)) {
  for (const via of v.via) {
    if (typeof via === "string") continue; // a dependency path; the advisory is listed under its own package
    const id = via.url?.split("/").pop() ?? `${name}: ${via.title}`;
    found.set(id, `${name} (${via.severity}): ${via.title}`);
  }
}

const today = new Date().toISOString().slice(0, 10);
const problems = [];
for (const [id, what] of found) {
  const exception = exceptions[id];
  if (!exception) problems.push(`${id} ${what}`);
  else if (exception.review < today) problems.push(`${id} ${what}: its exception was due for review on ${exception.review}`);
}
for (const id of Object.keys(exceptions)) {
  if (!found.has(id)) problems.push(`${id} no longer appears in npm audit; remove its exception`);
}

if (problems.length > 0) {
  console.error(`npm audit:\n  ${problems.join("\n  ")}`);
  process.exit(1);
}
console.log(`npm audit: no advisories beyond ${Object.keys(exceptions).length} reviewed exception(s).`);
