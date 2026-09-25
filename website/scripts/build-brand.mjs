// Renders the legacy Zinc spangle texture for older dashboards, plus favicons
// from the higher-definition site texture. Output is deterministic for these
// sources, so re-running the script only changes files when a source changes.
//
//   node scripts/build-brand.mjs

import sharp from "sharp";
import { fileURLToPath } from "node:url";

const out = (name) => fileURLToPath(new URL(`../public/${name}`, import.meta.url));

// Voronoi grains, each shaded along its own facet angle, like hot-dip zinc spangle.
function spangle(size, grains = 9, seed = 30) {
  let s = seed;
  const rnd = () => ((s = (s * 16807) % 2147483647) / 2147483647);
  const pts = Array.from({ length: grains }, () => ({
    x: rnd() * size,
    y: rnd() * size,
    base: 168 + rnd() * 62,
    a: rnd() * Math.PI * 2,
  }));
  const px = Buffer.alloc(size * size * 3);
  const edgeWidth = size / 38;

  for (let y = 0; y < size; y++) {
    for (let x = 0; x < size; x++) {
      let best = Infinity, second = Infinity, p = pts[0];
      for (const q of pts) {
        const d = (x - q.x) ** 2 + (y - q.y) ** 2;
        if (d < best) { second = best; best = d; p = q; }
        else if (d < second) second = d;
      }
      const facet = (((x - p.x) * Math.cos(p.a) + (y - p.y) * Math.sin(p.a)) / size) * 70;
      const edge = Math.sqrt(second) - Math.sqrt(best) < edgeWidth ? -18 : 0;
      const grain = (rnd() - 0.5) * 6;
      const sheen = (1 - (x + y) / (2 * size)) * 22;
      const v = Math.max(0, Math.min(255, p.base + facet + edge + grain + sheen));
      const o = (y * size + x) * 3;
      px[o] = Math.max(0, v - 6);
      px[o + 1] = Math.max(0, v - 2);
      px[o + 2] = Math.min(255, v + 4);
    }
  }
  return sharp(px, { raw: { width: size, height: size, channels: 3 } });
}

function label(size) {
  const sym = Math.round(size * 0.42);
  const num = Math.round(size * 0.2);
  return Buffer.from(`<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}">
  <text x="${size * 0.1}" y="${size * 0.24}" font-family="Menlo, monospace" font-size="${num}" fill="#2b3035">30</text>
  <text x="${size / 2}" y="${size * 0.84}" text-anchor="middle" font-family="Helvetica Neue, Arial, sans-serif" font-weight="700" font-size="${sym}" letter-spacing="-1" fill="#1e2226">Zn</text>
</svg>`);
}

await spangle(256).png().toFile(out("zn-spangle.png"));

for (const [name, size] of [["favicon.png", 64], ["apple-touch-icon.png", 180]]) {
  const base = await sharp(out("zn-spangle-hd.webp")).resize(size, size).png().toBuffer();
  await sharp(base).composite([{ input: label(size) }]).png().toFile(out(name));
}

console.log("brand assets written to public/");
