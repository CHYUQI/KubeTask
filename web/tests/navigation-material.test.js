import test, { before, after } from "node:test";
import assert from "node:assert/strict";
import { createServer } from "vite";
import { prepareNavigationElements } from "../src/components/navigation-material.js";

let server, buildBottomTabs, computeElementTransform, defaults, palette;
before(async () => {
  server = await createServer({
    server: { middlewareMode: true },
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
      208,
      () => {},
      defaults,
      () => {},
      null,
      palette,
    ).elements,
  );
}

test("navigation backing stays fixed through the upstream tab press and drag transforms", () => {
  const backing = elements().find((element) => element.isBottomTabContainer);
  assert.ok(
    backing,
    "keep the upstream marker needed for the glass backdrop snapshot",
  );
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
    assert.equal(transform.sx, backing.rect.x);
    assert.equal(transform.sy, backing.rect.y);
    assert.equal(transform.sw, backing.rect.w);
    assert.equal(transform.sh, backing.rect.h);
    assert.equal(transform.togglePressProgress, 0);
  }
});

test("only the selected indicator retains the original moving and elastic transform", () => {
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
