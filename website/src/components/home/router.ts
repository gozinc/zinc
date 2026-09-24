// A small model of Zinc's route matching, used by the homepage demo.
// It mirrors the documented rules: case-insensitive literals, non-strict
// trailing slashes, and static > parameter > catch-all precedence.

export type Kind = "static" | "param" | "catchall";

export interface Route {
  pattern: string;
  handler: string;
}

export interface Match {
  route: Route;
  params: [string, string][];
  kinds: Kind[];
}

export interface Result {
  path: string;
  winner: Match | null;
  others: Match[];
}

export const routes: Route[] = [
  { pattern: "/health", handler: "health" },
  { pattern: "/users/me", handler: "currentUser" },
  { pattern: "/users/{id}", handler: "showUser" },
  { pattern: "/users/{id}/posts/{post}", handler: "showPost" },
  { pattern: "/files/{path...}", handler: "serveFile" },
];

export const presets = ["/users/42/posts/7", "/users/me", "/Users/42/", "/files/css/app.css", "/teams/9"];

export const kindOf = (segment: string): Kind =>
  /^\{[^}]+\.\.\.\}$/.test(segment) ? "catchall" : /^\{[^}]+\}$/.test(segment) ? "param" : "static";

export const nameOf = (segment: string) => segment.replace(/^\{|\.\.\.\}$|\}$/g, "");

const split = (path: string) => path.split("/").filter((s, i, all) => i > 0 && !(s === "" && i === all.length - 1));

function tryMatch(route: Route, path: string): Match | null {
  const pattern = split(route.pattern);
  const parts = split(path);
  const params: [string, string][] = [];
  const kinds: Kind[] = [];

  for (let i = 0; i < pattern.length; i++) {
    const seg = pattern[i];
    const kind = kindOf(seg);
    kinds.push(kind);
    if (kind === "catchall") {
      params.push([nameOf(seg), parts.slice(i).join("/")]);
      return { route, params, kinds };
    }
    const part = parts[i];
    if (part === undefined) return null;
    if (kind === "param") {
      if (part === "") return null;
      params.push([nameOf(seg), decodeURIComponent(part)]);
    } else if (seg.toLowerCase() !== part.toLowerCase()) {
      return null;
    }
  }
  return parts.length === pattern.length ? { route, params, kinds } : null;
}

const rank: Record<Kind, number> = { static: 0, param: 1, catchall: 2 };

function compare(a: Match, b: Match) {
  for (let i = 0; i < Math.max(a.kinds.length, b.kinds.length); i++) {
    const d = rank[a.kinds[i] ?? "static"] - rank[b.kinds[i] ?? "static"];
    if (d !== 0) return d;
  }
  return 0;
}

export function resolve(input: string): Result {
  let path = input.trim() || "/";
  if (!path.startsWith("/")) path = `/${path}`;
  path = path.split(/[?#]/)[0];
  const matches = routes.map((r) => tryMatch(r, path)).filter((m): m is Match => m !== null).sort(compare);
  return { path, winner: matches[0] ?? null, others: matches.slice(1) };
}

export function explain(result: Result): string {
  const { winner, others } = result;
  if (!winner) return "No route matches, so the app-wide not-found handler responds.";
  if (others.length) {
    const loser = others[0].route.pattern;
    const why = winner.kinds.includes("param") || winner.kinds.includes("catchall")
      ? "parameters outrank catch-alls"
      : "static segments outrank parameters";
    return `Also matches ${loser}, but ${why}.`;
  }
  if (winner.kinds.includes("catchall")) return "The catch-all captures the rest of the path.";
  if (winner.params.length) return "Literals compare without case; captured values keep theirs.";
  return "An exact static match, the fastest path through the router.";
}

// Tree view of the routes, as the radix router sees them. Siblings are ordered
// by precedence (static, then parameter, then catch-all), so the order on
// screen is the order in which a request is matched.
export interface TreeRow {
  key: string; // cumulative pattern, e.g. "/users/{id}"; "" is the root
  seg: string;
  kind: Kind;
  guide: string; // box-drawing prefix
  handler?: string;
  name?: string; // parameter name, for captured values
}

export function treeRows(): TreeRow[] {
  interface Node { key: string; seg: string; kind: Kind; handler?: string; children: Node[] }
  const root: Node = { key: "", seg: "/", kind: "static", children: [] };
  for (const r of routes) {
    let node = root;
    let key = "";
    for (const seg of r.pattern.split("/").filter(Boolean)) {
      key += "/" + seg;
      let child = node.children.find((c) => c.key === key);
      if (!child) {
        child = { key, seg, kind: kindOf(seg), children: [] };
        node.children.push(child);
      }
      node = child;
    }
    node.handler = r.handler;
  }
  const order: Record<Kind, number> = { static: 0, param: 1, catchall: 2 };
  const rows: TreeRow[] = [];
  const walk = (n: Node, prefix: string, guide: string) => {
    rows.push({ key: n.key, seg: n.seg, kind: n.kind, guide, handler: n.handler, name: n.kind === "static" ? undefined : nameOf(n.seg) });
    const kids = [...n.children].sort((a, b) => order[a.kind] - order[b.kind]);
    kids.forEach((c, i) => {
      const last = i === kids.length - 1;
      walk(c, prefix + (last ? "   " : "│  "), prefix + (last ? "└─ " : "├─ "));
    });
  };
  walk(root, "", "");
  return rows;
}

// The tree keys a pattern passes through, root first.
export function pathKeys(pattern: string): string[] {
  const keys = [""];
  let key = "";
  for (const seg of pattern.split("/").filter(Boolean)) {
    key += "/" + seg;
    keys.push(key);
  }
  return keys;
}
