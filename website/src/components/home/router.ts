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

// A stray "%" must not break the demo, so undecodable values stay raw.
const decode = (part: string) => {
  try {
    return decodeURIComponent(part);
  } catch {
    return part;
  }
};

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
      params.push([nameOf(seg), decode(part)]);
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
  // Accept pasted URLs as well as paths: drop the origin, query, and fragment.
  let path = input.trim().replace(/^[a-z][a-z\d+.-]*:\/\/[^/]*/i, "").split(/[?#]/)[0] || "/";
  if (!path.startsWith("/")) path = `/${path}`;
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

// The request path split into segments, each labelled with the pattern part
// that consumed it. A catch-all consumes the rest of the path as one segment.
export interface Segment {
  text: string;
  kind: Kind | "none";
  label: string;
}

export function segments(result: Result): Segment[] {
  const parts = split(result.path);
  if (!parts.length) return [{ text: "/", kind: "none", label: result.winner ? "root" : "no route" }];
  const w = result.winner;
  if (!w) return parts.map((text) => ({ text: `/${text}`, kind: "none", label: "no match" }));
  const pattern = split(w.route.pattern);
  const out: Segment[] = [];
  for (let i = 0; i < pattern.length; i++) {
    const kind = kindOf(pattern[i]);
    if (kind === "catchall") {
      out.push({ text: `/${parts.slice(i).join("/")}`, kind, label: `{${nameOf(pattern[i])}...}` });
      break;
    }
    out.push({ text: `/${parts[i]}`, kind, label: kind === "param" ? `{${nameOf(pattern[i])}}` : "static" });
  }
  return out;
}

// The Go a reader would write to get this result, as highlighted tokens.
export type Token = [text: string, cls: "id" | "fn" | "str" | "pn" | "cm"];

export function codeLines(result: Result): Token[][] {
  const w = result.winner;
  const note: Token[] = [["// " + explain(result), "cm"]];
  if (!w) {
    return [[[`// No route matches GET ${result.path}.`, "cm"]], [["// The app's not-found handler answers 404.", "cm"]]];
  }
  const lines: Token[][] = [
    [["app", "id"], [".", "pn"], ["Get", "fn"], ["(", "pn"], [`"${w.route.pattern}"`, "str"], [", ", "pn"], [w.route.handler, "id"], [")", "pn"]],
  ];
  const width = Math.max(0, ...w.params.map(([n]) => n.length));
  for (const [name, value] of w.params) {
    lines.push([
      ["c", "id"], [".", "pn"], ["Param", "fn"], ["(", "pn"], [`"${name}"`, "str"], [")", "pn"],
      [" ".repeat(width - name.length + 1) + `// ${JSON.stringify(value)}`, "cm"],
    ]);
  }
  lines.push(note);
  return lines;
}
