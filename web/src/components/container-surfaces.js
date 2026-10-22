import { containerFrame, configureContainer } from "./container-material.js";
import { createGlassOutput } from "./glass-output.js";
import { createSurfaceCache, surfaceDpr } from "./surface-cache.js";

// Passive surfaces are baked once per size, then copied from a bounded bitmap
// cache. Opening a menu cannot repaint existing cards or resize a large atlas.
const entries = new Map();
const cache = createSurfaceCache();
let sequence = 0, renderer, sourceCanvas, output, template, corePromise;
let layoutFrame = 0, running = false, dirty = false, suspended = false;
let idleTimer, maxDpr = Math.min(window.devicePixelRatio || 1, 1.5);

async function initialize() {
  if (renderer || suspended) return;
  corePromise ||= Promise.all([
    import("@/vendor/liquid-glass-webgl/renderer/index.ts"),
    import("@/vendor/liquid-glass-webgl/catalog/build-scroll-container.ts"),
    import("@/vendor/liquid-glass-webgl/catalog/types.ts"),
  ]);
  const [core, catalog, types] = await corePromise;
  if (renderer || suspended || !entries.size) return;
  template = catalog.buildScrollContainer(
    400, () => {}, 1, types.DEFAULT_CATALOG_STATE, types.LIGHT_PALETTE,
  ).elements.find((element) => element.id === "sc-card-0");
  template.useSeparableBlur = types.DEFAULT_CATALOG_STATE.globalSeparableBlur;
  sourceCanvas = document.createElement("canvas");
  sourceCanvas.getContext("webgl", {
    alpha: true, premultipliedAlpha: true, antialias: false,
    preserveDrawingBuffer: false, powerPreference: "low-power",
  });
  renderer = new core.LiquidGlassRenderer(sourceCanvas);
  renderer.quickToggles.perElementFbo = true;
  renderer.backgroundColor = [245 / 255, 245 / 255, 247 / 255];
  maxDpr = Math.min(window.devicePixelRatio || 1, renderer.isSoftwareRenderer ? 1 : 1.5);
  output = createGlassOutput(renderer);
  const render = renderer.render.bind(renderer);
  renderer.render = () => {
    if (!renderer?.needsRedraw) return;
    output.begin();
    render();
    output.finish();
  };
  sourceCanvas.addEventListener("webglcontextlost", contextLost);
}

function schedule() {
  dirty = true;
  if (!layoutFrame && !running && !suspended && entries.size)
    layoutFrame = requestAnimationFrame(layout);
}

async function layout() {
  layoutFrame = 0;
  if (suspended) return;
  running = true;
  dirty = false;
  try {
    for (const entry of entries.values()) {
      if (!entry.visible || !entry.host.isConnected) continue;
      // Layout sizes, not transformed screen bounds: graph zoom must not scale
      // a node's material twice or allocate another bitmap on every zoom step.
      const width = entry.host.offsetWidth, height = entry.host.offsetHeight;
      if (width <= 0 || height <= 0) continue;
      let key = width + ":" + height + ":" + maxDpr;
      if (entry.key === key) continue;
      let baked = cache.get(key);
      if (!baked) {
        await initialize();
        if (!renderer || suspended || !entries.has(entry.id)) continue;
        key = width + ":" + height + ":" + maxDpr;
        baked = cache.get(key);
        if (!baked) {
          const frame = containerFrame(template, width, height);
          const dpr = surfaceDpr(frame.width, frame.height, maxDpr);
          renderer.dpr = dpr;
          renderer.resize(frame.width, frame.height);
          renderer.setElements([configureContainer(template, "static-surface",
            { ...frame, x: 0, y: 0 }, { width, height })]);
          renderer.render();
          const canvas = document.createElement("canvas");
          canvas.width = sourceCanvas.width;
          canvas.height = sourceCanvas.height;
          const context = canvas.getContext("2d");
          if (!context) continue;
          context.globalCompositeOperation = "copy";
          context.drawImage(sourceCanvas, 0, 0);
          baked = { canvas, frame, pixels: canvas.width * canvas.height };
          cache.set(key, baked);
        }
      }
      const { canvas, frame } = baked;
      entry.canvas.width = canvas.width;
      entry.canvas.height = canvas.height;
      Object.assign(entry.canvas.style, {
        left: -frame.padding + "px", top: -frame.padding + "px",
        width: frame.width + "px", height: frame.height + "px",
      });
      entry.context.globalCompositeOperation = "copy";
      entry.context.drawImage(canvas, 0, 0);
      entry.key = key;
      entry.onReady(true);
    }
  } catch {
    // Keep the semantic DOM and lightweight fallback if WebGL is unavailable.
    disposeRenderer();
  } finally {
    running = false;
    if (dirty) schedule();
    armIdleRelease();
  }
}

function armIdleRelease() {
  clearTimeout(idleTimer);
  // Keep compiled shaders while containers are mounted, but shrink idle GPU
  // buffers. A different menu size must not trigger shader compilation again.
  idleTimer = setTimeout(() => {
    if (!entries.size) { disposeRenderer(); return; }
    if (!renderer || running) return;
    renderer.setElements([]);
    renderer.resize(1, 1);
    renderer.ensureElementFBO(1, 1);
    renderer.render();
  }, 1500);
}
function disposeRenderer() {
  clearTimeout(idleTimer);
  const previous = renderer;
  renderer = undefined;
  sourceCanvas?.removeEventListener("webglcontextlost", contextLost);
  output?.dispose();
  output = undefined;
  previous?.dispose();
  previous?.gl.getExtension("WEBGL_lose_context")?.loseContext();
  if (sourceCanvas) sourceCanvas.width = sourceCanvas.height = 0;
  sourceCanvas = undefined;
}
function contextLost(event) {
  event.preventDefault();
  disposeRenderer();
}

const visibility = typeof IntersectionObserver === "undefined" ? null :
  new IntersectionObserver((changes) => {
    for (const change of changes) {
      for (const entry of entries.values()) {
        if (entry.host !== change.target) continue;
        entry.visible = change.isIntersecting;
        if (entry.visible) schedule();
        break;
      }
    }
  }, { rootMargin: "200px" });

export function registerContainerSurface(host, canvas, onReady) {
  const context = canvas.getContext("2d");
  if (!context) return { update() {}, dispose() {} };
  clearTimeout(idleTimer);
  const id = "app-container-" + ++sequence;
  entries.set(id, { id, host, canvas, context, onReady, visible: !visibility });
  visibility?.observe(host);
  schedule();
  return {
    update: schedule,
    dispose() {
      visibility?.unobserve(host);
      entries.delete(id);
      canvas.width = canvas.height = 0;
      if (!entries.size) {
        cancelAnimationFrame(layoutFrame);
        layoutFrame = 0;
        armIdleRelease();
      }
    },
  };
}

function hidePage() {
  suspended = true;
  cancelAnimationFrame(layoutFrame);
  layoutFrame = 0;
  disposeRenderer();
  cache.clear();
  for (const entry of entries.values()) {
    entry.key = undefined;
    entry.onReady(false);
  }
}
function showPage() {
  suspended = false;
  schedule();
}
window.addEventListener("pagehide", hidePage);
window.addEventListener("pageshow", showPage);
if (import.meta.hot) import.meta.hot.dispose(() => {
  hidePage();
  visibility?.disconnect();
  window.removeEventListener("pagehide", hidePage);
  window.removeEventListener("pageshow", showPage);
});
