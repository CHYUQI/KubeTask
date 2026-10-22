import { shadowBboxCss } from "../vendor/liquid-glass-webgl/renderer/methods-render-glass-geometry.ts";

// Geometry only: material, 32px corners, highlights and shadows come from
// the unchanged upstream Scroll Container card.
export function containerFrame(template, width, height) {
  const element = { ...template, rect: { x: 0, y: 0, w: width, h: height } };
  const box = shadowBboxCss(element, 0, 0, width, height, 1, 1, {
    outerShadow: true,
  });
  const padding = Math.ceil(Math.max(
    0, -box.x, -box.y, box.x + box.w - width, box.y + box.h - height,
  ) + 2);
  return {
    padding,
    width: Math.ceil(width + padding * 2),
    height: Math.ceil(height + padding * 2),
  };
}

export function configureContainer(template, id, cell, { width, height }) {
  return {
    ...template,
    id,
    cornerRadius: Math.min(template.cornerRadius, width / 2, height / 2),
    rect: {
      x: cell.x + cell.padding,
      y: cell.y + cell.padding,
      w: width,
      h: height,
    },
    scroll: false,
    clipRect: { x: cell.x, y: cell.y, w: cell.width, h: cell.height },
  };
}
