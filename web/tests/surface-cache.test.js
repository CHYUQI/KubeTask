import test from "node:test";
import assert from "node:assert/strict";
import { createSurfaceCache, surfaceDpr } from "../src/components/surface-cache.js";

const surface = (width, height) => ({ canvas: { width, height }, pixels: width * height });

test("equal-size passive cards reuse one bitmap and refresh its LRU position", () => {
  const cache = createSurfaceCache(200);
  const stats = surface(10, 10), menu = surface(10, 10), chart = surface(10, 10);
  cache.set("stats", stats);
  cache.set("menu", menu);
  assert.equal(cache.get("stats"), stats);
  cache.set("chart", chart);
  assert.equal(cache.size, 2);
  assert.equal(cache.pixels, 200);
  assert.equal(cache.get("menu"), undefined);
  assert.equal(menu.canvas.width, 0);
  assert.equal(cache.get("stats"), stats);
});

test("bitmap replacement and clearing release old storage without losing the new surface", () => {
  const cache = createSurfaceCache(300);
  const old = surface(10, 10), replacement = surface(20, 10);
  cache.set("panel", old);
  cache.set("panel", replacement);
  assert.equal(old.canvas.width, 0);
  assert.equal(cache.pixels, 200);
  assert.equal(cache.get("panel"), replacement);
  cache.clear();
  assert.equal(replacement.canvas.width, 0);
  assert.equal(cache.pixels, 0);
  assert.equal(cache.size, 0);
});

test("tall forms stay within the raster pixel budget without reducing small-card quality", () => {
  assert.equal(surfaceDpr(240, 172, 1.5), 1.5);
  for (const [width, height] of [[860, 1600], [1220, 4000], [2000, 10000]]) {
    const dpr = surfaceDpr(width, height, 1.5);
    assert.ok(width * height * dpr * dpr <= 2_000_000.001);
    assert.ok(dpr > 0 && dpr <= 1.5);
  }
});
