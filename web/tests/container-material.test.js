import test, { before, after } from "node:test";
import assert from "node:assert/strict";
import { createServer } from "vite";

let server, containerFrame, configureContainer, source;
before(async () => {
  server = await createServer({
    optimizeDeps: { noDiscovery: true, include: [] },
    server: { middlewareMode: true, hmr: false, ws: false },
    appType: "custom",
  });
  ({ containerFrame, configureContainer } = await server.ssrLoadModule(
    "/src/components/container-material.js",
  ));
  const { buildScrollContainer } = await server.ssrLoadModule(
    "/src/vendor/liquid-glass-webgl/catalog/build-scroll-container.ts",
  );
  const { DEFAULT_CATALOG_STATE, LIGHT_PALETTE } = await server.ssrLoadModule(
    "/src/vendor/liquid-glass-webgl/catalog/types.ts",
  );
  source = buildScrollContainer(400, () => {}, 1, DEFAULT_CATALOG_STATE, LIGHT_PALETTE)
    .elements.find((element) => element.id === "sc-card-0");
});
after(async () => { await server?.close(); });

test("dashboard panels retain the original Scroll Container material and corners", () => {
  const original = structuredClone(source);
  const cell = { ...containerFrame(source, 740, 390), x: 0, y: 0 };
  const panel = configureContainer(source, "trend", cell, { width: 740, height: 390 });
  for (const key of [
    "kind", "cornerRadius", "refractionHeight", "refractionAmount", "blurRadius",
    "saturation", "surfaceColor", "highlight", "outerShadow", "isInteractive",
    "independentBackdrop", "depthEffect",
  ]) assert.deepEqual(panel[key], source[key]);
  assert.equal(panel.cornerRadius, 32);
  assert.equal(panel.blurRadius, 0);
  assert.equal(panel.isInteractive, false);
  assert.equal(panel.scroll, false);
  assert.deepEqual(source, original);
});

test("statistics and large chart cards retain the complete native shadow bounds", () => {
  for (const [width, height] of [[128, 112], [280, 112], [740, 390], [1160, 300]]) {
    const cell = { ...containerFrame(source, width, height), x: 0, y: 0 };
    const { rect } = configureContainer(source, "card", cell, { width, height });
    const shadow = source.outerShadow;
    assert.ok(rect.x - shadow.radius + shadow.offsetX >= 0);
    assert.ok(rect.y - shadow.radius + shadow.offsetY >= 0);
    assert.ok(rect.x + width + shadow.radius + shadow.offsetX <= cell.width);
    assert.ok(rect.y + height + shadow.radius + shadow.offsetY <= cell.height);
  }
});

test("compact picker controls clamp the native corner radius to their geometry", () => {
  const cell = { ...containerFrame(source, 180, 38), x: 0, y: 0 };
  const picker = configureContainer(source, "picker", cell, { width: 180, height: 38 });
  assert.equal(picker.cornerRadius, 19);
  assert.deepEqual(picker.highlight, source.highlight);
  assert.deepEqual(picker.outerShadow, source.outerShadow);
});
