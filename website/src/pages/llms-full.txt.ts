import type { APIRoute } from "astro";
import { sections, summary, url } from "../lib/llms";

export const GET: APIRoute = async () => {
  const parts = [summary, ""];
  for (const section of await sections()) {
    for (const page of section.pages) {
      parts.push(`# ${page.data.title}`, "", `Source: ${url(page)}`, "", (page.body ?? "").trim(), "");
    }
  }
  return new Response(parts.join("\n"), { headers: { "Content-Type": "text/plain; charset=utf-8" } });
};
