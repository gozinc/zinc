// The middleware periodic table. Elements are numbered in the order they
// belong in a chain: each part wraps everything after it, so 1 runs first.
// Families are the rows, and each family is cast in its own metal.

export type Family = "observe" | "contain" | "shape" | "guard" | "carry";

export interface FamilyInfo {
  id: Family;
  name: string;
  metal: string;
  symbol: string;
  role: string;
}

export const families: FamilyInfo[] = [
  { id: "observe", name: "Observe", metal: "copper", symbol: "Cu", role: "Sees every request, so it wraps the rest" },
  { id: "contain", name: "Contain", metal: "titanium", symbol: "Ti", role: "Bounds panics, time, size, and load" },
  { id: "shape", name: "Shape", metal: "chromium", symbol: "Cr", role: "Normalises the URL before routing" },
  { id: "guard", name: "Guard", metal: "iron", symbol: "Fe", role: "Decides who gets in" },
  { id: "carry", name: "Carry", metal: "aluminium", symbol: "Al", role: "Encodes and serves the response" },
];

export interface Element {
  sym: string;
  name: string;
  fn: string;
  family: Family;
  slug: string;
  code: string; // the constructor call, as it appears in a chain
  http?: boolean; // standard net/http middleware, registered with UseHTTP
}

export const elements: Element[] = [
  { sym: "Ot", name: "OpenTelemetry", fn: "otelhttp", family: "observe", slug: "open-telemetry", code: 'otelhttp.NewMiddleware("api")', http: true },
  { sym: "Ri", name: "Request ID", fn: "requestid", family: "observe", slug: "requestid", code: "requestid.New()" },
  { sym: "Lg", name: "Request Logger", fn: "logger", family: "observe", slug: "logger", code: "logger.New()" },
  { sym: "Pm", name: "Prometheus", fn: "prometheus", family: "observe", slug: "prometheus", code: "prometheus.New()" },
  { sym: "Hc", name: "Health Check", fn: "healthcheck", family: "observe", slug: "healthcheck", code: "healthcheck.New()" },
  { sym: "Bd", name: "Body Dump", fn: "bodydump", family: "observe", slug: "bodydump", code: "bodydump.New(bodydump.Config{Observe: dump})" },

  { sym: "Rc", name: "Recover", fn: "recover", family: "contain", slug: "recover", code: "recover.New()" },
  { sym: "Ct", name: "Context Timeout", fn: "timeout", family: "contain", slug: "timeout", code: "timeout.New(timeout.Config{Timeout: 5 * time.Second})" },
  { sym: "Bl", name: "Body Limit", fn: "bodylimit", family: "contain", slug: "bodylimit", code: "bodylimit.New(bodylimit.Config{Limit: bodylimit.MB})" },
  { sym: "Rl", name: "Rate Limiter", fn: "limiter", family: "contain", slug: "limiter", code: "limiter.New(limiter.Config{Key: (*zinc.Context).IP})" },

  { sym: "Rd", name: "Redirect", fn: "redirect", family: "shape", slug: "redirect", code: 'redirect.New(redirect.Config{Rules: rules})' },
  { sym: "Ts", name: "Trailing Slash", fn: "trailingslash", family: "shape", slug: "trailingslash", code: "trailingslash.New()" },
  { sym: "Rw", name: "Rewrite", fn: "rewrite", family: "shape", slug: "rewrite", code: 'rewrite.New(rewrite.Config{Rules: rules})' },
  { sym: "Mo", name: "Method Override", fn: "methodoverride", family: "shape", slug: "methodoverride", code: "methodoverride.New()" },

  { sym: "Sh", name: "Secure Headers", fn: "secure", family: "guard", slug: "secure", code: "secure.New()" },
  { sym: "Co", name: "CORS", fn: "cors", family: "guard", slug: "cors", code: 'cors.New(cors.Config{AllowOrigins: origins})' },
  { sym: "Ty", name: "Content Type", fn: "contenttype", family: "guard", slug: "contenttype", code: 'contenttype.New(contenttype.Config{Types: types})' },
  { sym: "Ss", name: "Session", fn: "session", family: "guard", slug: "session", code: "session.New(session.Config{Secret: secret})" },
  { sym: "Cf", name: "CSRF", fn: "csrf", family: "guard", slug: "csrf", code: "csrf.New()" },
  { sym: "Ba", name: "Basic Auth", fn: "basicauth", family: "guard", slug: "basicauth", code: "basicauth.New(basicauth.Config{Validator: checkUser})" },
  { sym: "Ka", name: "Key Auth", fn: "keyauth", family: "guard", slug: "keyauth", code: "keyauth.New(keyauth.Config{Validator: checkKey})" },
  { sym: "Jw", name: "JWT", fn: "jwtauth", family: "guard", slug: "jwtauth", code: "jwtauth.New(jwtauth.Config{KeyFunc: keyFunc})" },
  { sym: "Cb", name: "Casbin", fn: "casbin", family: "guard", slug: "casbin", code: "casbin.New(casbin.Config{Enforcer: e, Subject: subject})" },

  { sym: "Dz", name: "Decompress", fn: "decompress", family: "carry", slug: "decompress", code: "decompress.New()" },
  { sym: "Cz", name: "Compress", fn: "compress", family: "carry", slug: "compress", code: "compress.New()" },
  { sym: "Nc", name: "No Cache", fn: "nocache", family: "carry", slug: "nocache", code: "nocache.New()" },
  { sym: "Hd", name: "Headers", fn: "headers", family: "carry", slug: "headers", code: "headers.New(headers.Config{Set: set})" },
  { sym: "Px", name: "Proxy", fn: "proxy", family: "carry", slug: "proxy", code: 'proxy.New(proxy.Config{Target: "http://backend:8080"})' },
  { sym: "Pp", name: "pprof", fn: "pprof", family: "carry", slug: "pprof", code: "pprof.New()" },
];

// Common compounds. Starter is the default stack from the middleware overview.
export const presets: { id: string; name: string; parts: string[] }[] = [
  { id: "starter", name: "Starter", parts: ["Ri", "Lg", "Rc", "Sh"] },
  { id: "api", name: "JSON API", parts: ["Ri", "Lg", "Rc", "Ct", "Bl", "Rl", "Sh", "Co", "Jw", "Cz"] },
  { id: "web", name: "Web app", parts: ["Ri", "Lg", "Rc", "Ts", "Sh", "Ss", "Cf", "Nc", "Cz"] },
  { id: "internal", name: "Internal service", parts: ["Ot", "Ri", "Lg", "Pm", "Rc", "Ct", "Ka", "Pp"] },
];

export const defaultPreset = presets[0];

/** The selected elements in chain order, whatever order they were picked in. */
export const inChainOrder = (syms: Iterable<string>) => {
  const set = new Set(syms);
  return elements.filter((e) => set.has(e.sym));
};

/** The Go that registers a compound. */
export function snippet(syms: Iterable<string>): string {
  const parts = inChainOrder(syms);
  if (!parts.length) return "// Select parts to build a stack.";
  const lines: string[] = [];
  for (const e of parts.filter((p) => p.http)) lines.push(`app.UseHTTP(${e.code})`);
  const zinc = parts.filter((p) => !p.http);
  if (zinc.length === 1) lines.push(`app.Use(${zinc[0].code})`);
  else if (zinc.length) lines.push("app.Use(", ...zinc.map((e) => `\t${e.code},`), ")");
  return lines.join("\n");
}

export type TokenKind = "fn" | "str" | "num" | "punct" | "prop" | "id" | "comment" | "ws";

/** A small Go highlighter for the generated snippet, in the zinc-frost palette. */
export function tokenize(code: string): { text: string; kind: TokenKind }[] {
  const out: { text: string; kind: TokenKind }[] = [];
  const re = /(\/\/[^\n]*)|("[^"]*")|(\d+)|([A-Za-z_]\w*)|(\s+)|([^\w\s"]+)/g;
  let m: RegExpExecArray | null;
  while ((m = re.exec(code))) {
    const [text, comment, str, num, id, ws] = m;
    let kind: TokenKind = "punct";
    if (comment) kind = "comment";
    else if (str) kind = "str";
    else if (num) kind = "num";
    else if (ws) kind = "ws";
    else if (id) kind = code[re.lastIndex] === "(" ? "fn" : code[m.index - 1] === "." ? "prop" : "id";
    out.push({ text, kind });
  }
  return out;
}
