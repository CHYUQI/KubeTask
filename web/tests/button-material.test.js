import test, { before, after } from "node:test";
import assert from "node:assert/strict";
import { createServer } from "vite";

let server,
  buttonFrame,
  packButtonFrames,
  configureButton,
  buttonPointerPosition,
  buildButtons,
  types,
  computeElementTransform,
  shadowBboxCss;
before(async () => {
  server = await createServer({
    optimizeDeps: { noDiscovery: true, include: [] },
    server: { middlewareMode: true, hmr: false, ws: false },
    appType: "custom",
  });
  ({ buttonFrame, packButtonFrames, configureButton, buttonPointerPosition } =
    await server.ssrLoadModule("/src/components/button-material.js"));
  ({ buildButtons } = await server.ssrLoadModule(
    "/src/vendor/liquid-glass-webgl/catalog/build-buttons.ts",
  ));
  types = await server.ssrLoadModule(
    "/src/vendor/liquid-glass-webgl/catalog/types.ts",
  );
  ({ computeElementTransform } = await server.ssrLoadModule(
    "/src/vendor/liquid-glass-webgl/renderer/methods-render-glass-transform.ts",
  ));
  ({ shadowBboxCss } = await server.ssrLoadModule(
    "/src/vendor/liquid-glass-webgl/renderer/methods-render-glass-geometry.ts",
  ));
});
after(async () => {
  await server?.close();
});

function template() {
  return buildButtons(
    600,
    600,
    () => {},
    types.DEFAULT_CATALOG_STATE,
    types.LIGHT_PALETTE,
  ).elements.find((element) => element.id === "btn-tinted-blue");
}

test("button adaptation preserves upstream material, shadow and foreground animation", () => {
  const source = template();
  const frame = { ...buttonFrame(112, 38, types.DEFAULT_SHADOW), x: 0, y: 0 };
  const button = configureButton(source, "create", frame, {
    width: 112,
    height: 38,
    label: "创建工作流",
    fontSize: 14,
  });
  for (const key of [
    "refractionHeight",
    "refractionAmount",
    "blurRadius",
    "saturation",
    "highlight",
    "outerShadow",
    "tintColor",
    "surfaceColor",
    "isInteractive",
  ])
    assert.deepEqual(button[key], source[key]);
  assert.equal(button.label, "创建工作流");
  assert.equal(button.kind, "button");
  assert.equal(button.cornerRadius, 19);
  assert.equal(source.label, "Tinted Liquid Button");
});

test("native button shadow fits at rest, press overshoot and diagonal drag", () => {
  for (const [width, height] of [
    [64, 38],
    [144, 44],
    [320, 48],
  ]) {
    const cell = {
      ...buttonFrame(width, height, types.DEFAULT_SHADOW),
      x: 0,
      y: 0,
    };
    const element = configureButton(template(), "button", cell, {
      width,
      height,
      label: "操作",
      fontSize: 14,
    });
    for (const pressProgress of [-0.17, 0, 1, 1.2]) {
      for (const dragX of [-width, 0, width]) {
        const transform = computeElementTransform.call(
          {},
          element,
          { pressProgress, dragX, dragY: height, startDragX: 0, startDragY: 0 },
          element.rect,
        );
        const box = shadowBboxCss(
          element,
          transform.sx,
          transform.sy,
          transform.sw,
          transform.sh,
          transform.scaleX,
          transform.scaleY,
          { outerShadow: true },
        );
        assert.ok(
          box.x >= 0 &&
            box.y >= 0 &&
            box.x + box.w <= cell.width &&
            box.y + box.h <= cell.height,
        );
      }
    }
  }
});

test("shared button cells do not overlap or clip their own shadows", () => {
  const frames = [64, 144, 100, 320, 96, 64].map((width) =>
    buttonFrame(width, 38, types.DEFAULT_SHADOW),
  );
  const { cells, width, height } = packButtonFrames(frames, 700);
  for (let i = 0; i < cells.length; i++) {
    const a = cells[i];
    assert.ok(a.x + a.width <= width && a.y + a.height <= height);
    for (const b of cells.slice(i + 1))
      assert.ok(
        a.x + a.width <= b.x ||
          b.x + b.width <= a.x ||
          a.y + a.height <= b.y ||
          b.y + b.height <= a.y,
      );
  }
});

test("captured touch/pen drags outside the button keep the animated shadow inside its frame", () => {
  const width = 96.5, height = 38;
  const frame = { ...buttonFrame(width, height, types.DEFAULT_SHADOW), x: 0, y: 0 };
  const element = configureButton(template(), "captured", frame, {
    width, height, label: "更多", fontSize: 14,
  });
  const bounds = { left: 240, top: 100, width, height };
  for (const start of [{ x: 0, y: height }, { x: width, y: 0 }]) {
    for (const clientX of [-1000, 2000]) {
      for (const clientY of [-1000, 2000]) {
        const point = buttonPointerPosition(bounds, { clientX, clientY });
        assert.ok(point.x >= 0 && point.x <= width && point.y >= 0 && point.y <= height);
        for (const pressProgress of [-0.17, 0, 1, 1.25]) {
          const transform = computeElementTransform.call({}, element, {
            pressProgress, dragX: point.x, dragY: point.y,
            startDragX: start.x, startDragY: start.y,
          }, element.rect);
          const box = shadowBboxCss(element, transform.sx, transform.sy,
            transform.sw, transform.sh, transform.scaleX, transform.scaleY,
            { outerShadow: true });
          assert.ok(box.x >= 0 && box.y >= 0 &&
            box.x + box.w <= frame.width && box.y + box.h <= frame.height);
        }
      }
    }
  }
});
