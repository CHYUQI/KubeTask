import { computeElementTransform } from "../vendor/liquid-glass-webgl/renderer/methods-render-glass-transform.ts";
import { shadowBboxCss } from "../vendor/liquid-glass-webgl/renderer/methods-render-glass-geometry.ts";

// Touch/pen implicitly capture pointers and can report positions far outside
// the DOM button. Keep the upstream drag transform within its allocated frame.
export function buttonPointerPosition(bounds, point) {
  return {
    x: Math.max(0, Math.min(bounds.width, point.clientX - bounds.left)),
    y: Math.max(0, Math.min(bounds.height, point.clientY - bounds.top)),
  };
}

// Only layout and application labels are adapted here. Optical parameters,
// shadows, foreground drawing and press physics stay in the upstream source.
export function buttonFrame(width, height, shadow) {
  const element = {
    kind: "button",
    isInteractive: true,
    rect: { x: 0, y: 0, w: width, h: height },
    cornerRadius: height / 2,
    outerShadow: shadow,
  };
  let reach = 0;
  // Measure the original transform and shadow, including spring overshoot
  // and dragging across the button. No guessed CSS shadow or 16px crop.
  for (const pressProgress of [0, 1.25]) {
    for (const dragX of [-width, 0, width]) {
      for (const dragY of [-height, 0, height]) {
        const transform = computeElementTransform.call(
          {},
          element,
          { pressProgress, dragX, dragY, startDragX: 0, startDragY: 0 },
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
        reach = Math.max(
          reach,
          -box.x,
          -box.y,
          box.x + box.w - width,
          box.y + box.h - height,
        );
      }
    }
  }
  const padding = Math.ceil(reach + 2);
  return {
    padding,
    width: Math.ceil(width + padding * 2),
    height: Math.ceil(height + padding * 2),
  };
}

export function packButtonFrames(frames, maxWidth = 1024) {
  let x = 0,
    y = 0,
    rowHeight = 0,
    width = 1;
  const cells = frames.map((frame) => {
    if (x && x + frame.width > maxWidth) {
      y += rowHeight;
      x = 0;
      rowHeight = 0;
    }
    const cell = { ...frame, x, y };
    x += frame.width;
    width = Math.max(width, x);
    rowHeight = Math.max(rowHeight, frame.height);
    return cell;
  });
  return { cells, width, height: Math.max(1, y + rowHeight) };
}

export function configureButton(template, id, cell, description) {
  return {
    ...template,
    id,
    rect: {
      x: cell.x + cell.padding,
      y: cell.y + cell.padding,
      w: description.width,
      h: description.height,
    },
    cornerRadius: description.height / 2,
    label: description.label,
    labelFontSizePx: description.fontSize,
    scroll: false,
    clipRect: { x: cell.x, y: cell.y, w: cell.width, h: cell.height },
  };
}
