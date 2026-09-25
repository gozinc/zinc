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
  { sym: "Ri", name: "Request ID", fn: "RequestID", family: "observe", slug: "request-id", code: "middleware.RequestID()" },
  { sym: "Lg", name: "Request Logger", fn: "RequestLogger", family: "observe", slug: "request-logger", code: "middleware.RequestLogger()" },
  { sym: "Pm", name: "Prometheus", fn: "Prometheus", family: "observe", slug: "prometheus", code: "middleware.Prometheus()" },
  { sym: "Jg", name: "Jaeger", fn: "Jaeger", family: "observe", slug: "jaeger", code: "middleware.Jaeger(observer)" },
  { sym: "Bd", name: "Body Dump", fn: "BodyDump", family: "observe", slug: "body-dump", code: "middleware.BodyDump(dump)" },

  { sym: "Rc", name: "Recover", fn: "Recover", family: "contain", slug: "recover", code: "middleware.Recover()" },
  { sym: "Ct", name: "Context Timeout", fn: "ContextTimeout", family: "contain", slug: "context-timeout", code: "middleware.ContextTimeout(5 * time.Second)" },
  { sym: "Bl", name: "Body Limit", fn: "BodyLimit", family: "contain", slug: "body-limit", code: "middleware.BodyLimit(1 << 20)" },
  { sym: "Rl", name: "Rate Limiter", fn: "IPRateLimiter", family: "contain", slug: "rate-limiter", code: "middleware.IPRateLimiter(10, 20)" },
  { sym: "Ut", name: "Utility", fn: "Throttle", family: "contain", slug: "utility", code: "middleware.Throttle(100)" },

  { sym: "Rd", name: "Redirect", fn: "Redirect", family: "shape", slug: "redirect", code: 'middleware.Redirect("/old", "/new")' },
  { sym: "Ts", name: "Trailing Slash", fn: "TrailingSlash", family: "shape", slug: "trailing-slash", code: "middleware.TrailingSlash()" },
  { sym: "Rw", name: "Rewrite", fn: "Rewrite", family: "shape", slug: "rewrite", code: 'middleware.Rewrite("/v1/users", "/users")' },
  { sym: "Mo", name: "Method Override", fn: "MethodOverride", family: "shape", slug: "method-override", code: "middleware.MethodOverride()" },

  { sym: "Sh", name: "Secure Headers", fn: "Secure", family: "guard", slug: "secure", code: "middleware.Secure()" },
  { sym: "Co", name: "CORS", fn: "CORS", family: "guard", slug: "cors", code: 'middleware.CORS("https://app.example.com")' },
  { sym: "Hg", name: "Header Guards", fn: "AllowContentType", family: "guard", slug: "header-guards", code: 'middleware.AllowContentType("application/json")' },
  { sym: "Ss", name: "Session", fn: "SessionCookie", family: "guard", slug: "session", code: 'middleware.SessionCookie("session", secret)' },
  { sym: "Cf", name: "CSRF", fn: "CSRF", family: "guard", slug: "csrf", code: "middleware.CSRF()" },
  { sym: "Ba", name: "Basic Auth", fn: "BasicAuth", family: "guard", slug: "basic-auth", code: "middleware.BasicAuth(checkUser)" },
  { sym: "Ka", name: "Key Auth", fn: "KeyAuth", family: "guard", slug: "key-auth", code: "middleware.KeyAuth(checkKey)" },
  { sym: "Jw", name: "JWT", fn: "JWT", family: "guard", slug: "jwt", code: "middleware.JWT(keyFunc)" },
  { sym: "Cb", name: "Casbin", fn: "CasbinAuth", family: "guard", slug: "casbin-auth", code: "middleware.CasbinAuth(enforcer, subject)" },

  { sym: "Dz", name: "Decompress", fn: "Decompress", family: "carry", slug: "decompress", code: "middleware.Decompress()" },
  { sym: "Gz", name: "Gzip", fn: "Gzip", family: "carry", slug: "gzip", code: "middleware.Gzip()" },
  { sym: "St", name: "Static", fn: "Static", family: "carry", slug: "static", code: 'middleware.Static("./public")' },
  { sym: "Px", name: "Proxy", fn: "Proxy", family: "carry", slug: "proxy", code: 'middleware.Proxy("http://backend:8080")' },
  { sym: "Pp", name: "pprof", fn: "Pprof", family: "carry", slug: "pprof", code: "middleware.Pprof()" },
];

// Common compounds. Starter is the default stack from the middleware overview.
export const presets: { id: string; name: string; parts: string[] }[] = [
  { id: "starter", name: "Starter", parts: ["Ri", "Lg", "Rc", "Sh"] },
  { id: "api", name: "JSON API", parts: ["Ri", "Lg", "Rc", "Ct", "Bl", "Rl", "Sh", "Co", "Jw", "Gz"] },
  { id: "web", name: "Web app", parts: ["Ri", "Lg", "Rc", "Ts", "Sh", "Ss", "Cf", "Gz", "St"] },
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
