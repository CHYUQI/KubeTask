import {
  buttonFrame,
  packButtonFrames,
  configureButton,
} from "./button-material.js";
import { createGlassOutput } from "./glass-output.js";

// One upstream renderer serves the DOM buttons. Copy its complete frames to
// presentation canvases; CSS does not synthesize shadows or press animations.
// Keep it across routes to avoid compiling the same shaders per button.
const entries = new Map();
let sequence = 0,
  renderer,
  sourceCanvas,
  corePromise,
  buildButtons,
  makePlainRect,
  types,
  output;
let layoutRaf = 0,
  closed = false;

async function initialize() {
  if (renderer || closed) return;
  corePromise ||= Promise.all([
    import("@/vendor/liquid-glass-webgl/renderer/index.ts"),
    import("@/vendor/liquid-glass-webgl/catalog/build-buttons.ts"),
    import("@/vendor/liquid-glass-webgl/catalog/helpers-elements.ts"),
    import("@/vendor/liquid-glass-webgl/catalog/types.ts"),
  ]);
  const [core, buttons, helpers, catalogTypes] = await corePromise;
  if (renderer || closed || !entries.size) return;
  buildButtons = buttons.buildButtons;
  makePlainRect = helpers.makePlainRect;
  types = catalogTypes;
  sourceCanvas = document.createElement("canvas");
  // Context attributes apply on first acquisition. Keep upstream's context
  // settings, with an alpha channel for DOM composition of its native passes.
  sourceCanvas.getContext("webgl", {
    alpha: true,
    premultipliedAlpha: true,
    antialias: false,
    preserveDrawingBuffer: false,
    powerPreference: "low-power",
  });
  renderer = new core.LiquidGlassRenderer(sourceCanvas);
  renderer.quickToggles.perElementFbo = true;
  renderer.backgroundColor = [245 / 255, 245 / 255, 247 / 255];
  renderer.dpr = Math.min(
    window.devicePixelRatio || 1,
    renderer.isSoftwareRenderer ? 1.5 : 2,
  );
  output = createGlassOutput(renderer);
  const render = renderer.render.bind(renderer);
  renderer.render = () => {
    if (!renderer?.needsRedraw) return;
    output.begin();
    render();
    output.finish();
    present();
  };
  sourceCanvas.addEventListener("webglcontextlost", contextLost);
  scheduleLayout();
}

function scheduleLayout() {
  if (!layoutRaf && !closed) layoutRaf = requestAnimationFrame(layout);
}

function layout() {
  layoutRaf = 0;
  if (!renderer) return;
  const active = [];
  for (const entry of entries.values()) {
    const description = entry.describe();
    if (description.width <= 0 || description.height <= 0) {
      entry.cell = undefined;
      entry.onReady(false);
      continue;
    }
    entry.description = description;
    entry.dirty = true;
    active.push(entry);
  }
  const packed = packButtonFrames(
    active.map((entry) =>
      buttonFrame(
        entry.description.width,
        entry.description.height,
        types.DEFAULT_SHADOW,
      ),
    ),
  );
  const templates = buildButtons(
    600,
    600,
    () => {},
    types.DEFAULT_CATALOG_STATE,
    types.LIGHT_PALETTE,
  ).elements;
  const elements = [];
  active.forEach((entry, index) => {
    const cell = packed.cells[index];
    entry.cell = cell;
    const { description } = entry;
    const template = templates.find(
      (element) =>
        element.id ===
        (description.tone === "accent" ? "btn-tinted-blue" : "btn-surface"),
    );
    // Apply the same catalog-wide default as upstream buildCatalog.
    template.useSeparableBlur = types.DEFAULT_CATALOG_STATE.globalSeparableBlur;
    // The catalog owns the full canvas background. Match each cell to the
    // solid surface where its DOM button lives, including the shadow margin.
    elements.push(
      makePlainRect(
        `${entry.id}-background`,
        { x: cell.x, y: cell.y, w: cell.width, h: cell.height },
        [...description.background, 1],
        0,
        false,
      ),
    );
    elements.push(configureButton(template, entry.id, cell, description));
    Object.assign(entry.canvas.style, {
      left: `${-cell.padding}px`,
      top: `${-cell.padding}px`,
      width: `${cell.width}px`,
      height: `${cell.height}px`,
    });
  });
  if (
    renderer.cssWidth !== packed.width ||
    renderer.cssHeight !== packed.height
  )
    renderer.resize(packed.width, packed.height);
  renderer.setElements(elements);
  renderer.requestRender();
}

function present() {
  for (const entry of entries.values()) {
    if (!entry.cell) continue;
    const state = renderer.buttonStates.get(entry.id);
    const signature = state && [state.pressProgress, state.dragX, state.dragY,
      state.startDragX, state.startDragY].join(":");
    if (!entry.dirty && entry.signature === signature) continue;
    entry.signature = signature;
    entry.dirty = false;
    const { x, y, width, height } = entry.cell;
    const dpr = renderer.dpr;
    const w = Math.round(width * dpr),
      h = Math.round(height * dpr);
    if (entry.canvas.width !== w) entry.canvas.width = w;
    if (entry.canvas.height !== h) entry.canvas.height = h;
    entry.context.globalCompositeOperation = "copy";
    entry.context.drawImage(
      sourceCanvas,
      x * dpr,
      y * dpr,
      width * dpr,
      height * dpr,
      0,
      0,
      w,
      h,
    );
    // Reveal only after a complete source frame, matching upstream onReady.
    entry.onReady(true);
  }
}

function contextLost(event) {
  event.preventDefault();
  output?.dispose();
  renderer?.dispose();
  renderer = undefined;
  for (const entry of entries.values()) entry.onReady(false);
}

export function registerButtonSurface(canvas, describe, onReady) {
  const id = `app-button-${++sequence}`;
  const context = canvas.getContext("2d");
  if (!context) return { update() {}, press() {}, drag() {}, dispose() {} };
  const entry = { id, canvas, context, describe, onReady };
  entries.set(id, entry);
  initialize().catch(() => {
    for (const item of entries.values()) item.onReady(false);
  });
  scheduleLayout();
  return {
    update: scheduleLayout,
    press(pressed, position) {
      if (!renderer || !entry.cell) return;
      if (!pressed && !entry.pressed) return;
      entry.pressed = pressed;
      renderer.setPressed(
        id,
        pressed,
        position && {
          x: entry.cell.x + entry.cell.padding + position.x,
          y: entry.cell.y + entry.cell.padding + position.y,
        },
      );
    },
    drag(position) {
      if (!renderer || !entry.cell) return;
      renderer.setDragPosition(id, {
        x: entry.cell.x + entry.cell.padding + position.x,
        y: entry.cell.y + entry.cell.padding + position.y,
      });
    },
    dispose() {
      if (entry.pressed) renderer?.setPressed(id, false);
      entries.delete(id);
      scheduleLayout();
    },
  };
}

// No perpetual presentation loop: only upstream requestRender drives frames.
if (typeof window !== "undefined") {
  window.addEventListener("pagehide", () => {
    closed = true;
    cancelAnimationFrame(layoutRaf);
    layoutRaf = 0;
    const previous = renderer;
    renderer = undefined;
    output?.dispose();
    previous?.dispose();
    previous?.gl.getExtension("WEBGL_lose_context")?.loseContext();
    for (const entry of entries.values()) entry.onReady(false);
  });
  window.addEventListener("pageshow", () => {
    closed = false;
    if (entries.size) initialize().catch(() => {});
  });
}
