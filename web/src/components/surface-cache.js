// A bounded LRU for baked, passive glass surfaces. DOM presentation canvases
// keep their own copy, so evicting a cached surface cannot erase visible UI.
export function createSurfaceCache(maxPixels = 4_000_000) {
  const values = new Map();
  let pixels = 0;
  function release(value) {
    pixels -= value.pixels;
    value.canvas.width = value.canvas.height = 0;
  }
  return {
    get(key) {
      const value = values.get(key);
      if (value) {
        values.delete(key);
        values.set(key, value);
      }
      return value;
    },
    set(key, value) {
      if (values.has(key)) release(values.get(key));
      values.delete(key);
      values.set(key, value);
      pixels += value.pixels;
      while (pixels > maxPixels && values.size > 1) {
        const oldest = values.keys().next().value;
        release(values.get(oldest));
        values.delete(oldest);
      }
    },
    clear() {
      for (const value of values.values()) release(value);
      values.clear();
    },
    get pixels() { return pixels; },
    get size() { return values.size; },
  };
}

// Bound the actual FBO/bitmap pixels for a tall form, independently of its DOM
// text resolution. Large surfaces keep the original CSS-space glass geometry.
export function surfaceDpr(width, height, deviceDpr, maxPixels = 2_000_000) {
  return Math.min(deviceDpr, Math.sqrt(maxPixels / (width * height)));
}
