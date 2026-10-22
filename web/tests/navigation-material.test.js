import test, { before, after } from "node:test";
import assert from "node:assert/strict";
import { createServer } from "vite";
import {
  prepareNavigationElements,
  NAVIGATION_CATALOG_HEIGHT,
  NAVIGATION_FRAME_HEIGHT,
} from "../src/components/navigation-material.js";

let server, buildBottomTabs, computeElementTransform, defaults, palette;
before(async () => {
  server = await createServer({
    optimizeDeps: { noDiscovery: true, include: [] },
    server: { middlewareMode: true, hmr: false, ws: false },
    appType: "custom",
  });
  ({ buildBottomTabs } = await server.ssrLoadModule(
    "/src/vendor/liquid-glass-webgl/catalog/build-bottom-tabs.ts",
  ));
  ({ computeElementTransform } = await server.ssrLoadModule(
    "/src/vendor/liquid-glass-webgl/renderer/methods-render-glass-transform.ts",
  ));
  const types = await server.ssrLoadModule(
    "/src/vendor/liquid-glass-webgl/catalog/types.ts",
  );
  defaults = types.DEFAULT_CATALOG_STATE;
  palette = types.LIGHT_PALETTE;
});
after(async () => {
  await server?.close();
});

function elements() {
  return prepareNavigationElements(
    buildBottomTabs(
      456,
      NAVIGATION_CATALOG_HEIGHT,
      () => {},
      defaults,
      () => {},
      null,
      palette,
    ).elements,
  );
}

test("navigation backing retains the upstream group, material and press/drag transforms", () => {
  const backing = elements().find((element) => element.isBottomTabContainer);
  assert.ok(
    backing,
    "keep the upstream marker needed for the glass backdrop snapshot",
  );
  const source = buildBottomTabs(
    456,
    NAVIGATION_CATALOG_HEIGHT,
    () => {},
    defaults,
    () => {},
    null,
    palette,
  ).elements.find((element) => element.id === backing.id);
  assert.deepEqual(backing, source);
  assert.equal(backing.isBottomTabContainer.groupId, "tabs3");
  for (const fraction of [0, 0.35, 1, 1.7, 2]) {
    const renderer = {
      toggleStates: new Map([
        [
          "tabs3",
          {
            fraction,
            pressProgress: 1,
            scaleX: 78 / 56,
            scaleY: 78 / 56,
            velocity: 4,
            panelOffset: 3,
          },
        ],
      ]),
    };
    const transform = computeElementTransform.call(
      renderer,
      backing,
      undefined,
      backing.rect,
    );
    const original = computeElementTransform.call(
      renderer,
      source,
      undefined,
      source.rect,
    );
    assert.deepEqual(transform, original);
    assert.ok(transform.sw > backing.rect.w);
    assert.equal(transform.togglePressProgress, 1);
  }
});

test("cropped navigation frame retains the visible material and backing shadow", () => {
  const visible = elements();
  const backing = visible.find((element) => element.isBottomTabContainer);
  assert.ok(NAVIGATION_FRAME_HEIGHT < NAVIGATION_CATALOG_HEIGHT);
  for (const element of visible) {
    assert.ok(element.rect.y >= 0);
    assert.ok(element.rect.y + element.rect.h <= NAVIGATION_FRAME_HEIGHT);
  }
  assert.ok(
    backing.rect.y + backing.rect.h + backing.outerShadow.radius <=
      NAVIGATION_FRAME_HEIGHT,
  );
});

test("selected indicator retains the original moving and elastic transform", () => {
  const indicator = elements().find((element) => element.isBottomTabIndicator);
  assert.equal(indicator.isBottomTabIndicator.groupId, "tabs3");
  const renderer = {
    toggleStates: new Map([
      [
        "tabs3",
        {
          fraction: 1.25,
          pressProgress: 1,
          scaleX: 78 / 56,
          scaleY: 78 / 56,
          velocity: 0,
          panelOffset: 0,
        },
      ],
    ]),
  };
  const transform = computeElementTransform.call(
    renderer,
    indicator,
    undefined,
    indicator.rect,
  );
  assert.ok(
    transform.sw > indicator.rect.w,
    "preserve the upstream press enlargement",
  );
  assert.equal(
    transform.sx + transform.sw / 2,
    indicator.rect.x +
      indicator.rect.w / 2 +
      1.25 * indicator.isBottomTabIndicator.dragWidth,
  );
});
