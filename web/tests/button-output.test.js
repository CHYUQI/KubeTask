import test from "node:test";
import assert from "node:assert/strict";
import { createGlassOutput } from "../src/components/glass-output.js";

for (const bodyOpacity of [1, 0.22]) test(`DOM output preserves shadow/highlight with body opacity ${bodyOpacity}`, () => {
  // Three sample pixels: shadow, button body, and an additive rim fragment
  // whose RGB is zero. The source assumes an opaque wallpaper under all three.
  const buffers = new Map([
    [null, Array.from({ length: 3 }, () => [0, 0, 0, 0])],
  ]);
  let bound = null,
    factors = [1, 0, 1, 0],
    deleted = 0, constantAlpha = 1;
  const gl = {
    ZERO: 0,
    ONE: 1,
    SRC_ALPHA: 2,
    ONE_MINUS_SRC_ALPHA: 3,
    FRAMEBUFFER: 4,
    SCISSOR_TEST: 5,
    COLOR_BUFFER_BIT: 6,
    CONSTANT_ALPHA: 7,
    blendColor(_r, _g, _b, alpha) { constantAlpha = alpha; },
    bindFramebuffer(_kind, fb) {
      bound = fb;
    },
    blendFunc(src, dst) {
      factors = [src, dst, src, dst];
    },
    blendFuncSeparate(...values) {
      factors = values;
    },
    disable() {},
    clearColor() {},
    clear() {
      buffers.set(
        bound,
        Array.from({ length: 3 }, () => [0, 0, 0, 0]),
      );
    },
    deleteFramebuffer() {
      deleted++;
    },
    deleteTexture() {
      deleted++;
    },
  };
  function draw(index, source) {
    const factor = (value) =>
      value === gl.ONE
        ? 1
        : value === gl.ZERO
          ? 0
          : value === gl.CONSTANT_ALPHA
            ? constantAlpha
          : value === gl.SRC_ALPHA
            ? source[3]
            : 1 - source[3];
    const destination = buffers.get(bound)[index];
    buffers.get(bound)[index] = source.map((value, channel) =>
      Math.min(
        1,
        value * factor(factors[channel === 3 ? 2 : 0]) +
          destination[channel] * factor(factors[channel === 3 ? 3 : 1]),
      ),
    );
  }
  const scene = {};
  buffers.set(
    scene,
    Array.from({ length: 3 }, () => [1, 1, 1, 1]),
  );
  const renderer = {
    gl,
    fboW: 3,
    fboH: 1,
    createFBO() {
      const fb = {};
      buffers.set(fb, []);
      return { fb, tex: fb };
    },
    bindFBO(fb) {
      gl.bindFramebuffer(gl.FRAMEBUFFER, fb);
    },
    drawCopy(tex) {
      buffers.set(
        bound,
        buffers.get(tex).map((pixel) => [...pixel]),
      );
    },
    renderGlassShadowPass() {
      gl.blendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA);
      draw(0, [0, 0, 0, 0.1]);
    },
    drawElFboComposite() {
      gl.blendFuncSeparate(
        gl.ONE,
        gl.ONE_MINUS_SRC_ALPHA,
        gl.ONE,
        gl.ONE_MINUS_SRC_ALPHA,
      );
      draw(1, [0, 0.5, 1, 1]);
    },
    renderGlassPostPasses() {
      gl.blendFunc(gl.ONE, gl.ONE);
      draw(2, [0, 0, 0, 1]);
      gl.blendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA);
    },
    renderTextElement(_element, _rect, _state, destination) {
      this.bindFBO(destination);
      gl.blendFunc(gl.ONE, gl.ONE_MINUS_SRC_ALPHA);
      draw(2, [0.2, 0.3, 0.4, 1]);
      return true;
    },
  };
  const originalShadow = renderer.renderGlassShadowPass;
  const originalText = renderer.renderTextElement;
  const output = createGlassOutput(renderer, { bodyOpacity, mirrorText: true });
  output.begin();
  renderer.bindFBO(scene);
  renderer.renderGlassShadowPass();
  renderer.drawElFboComposite();
  renderer.renderGlassPostPasses();
  assert.deepEqual(factors, [
    gl.SRC_ALPHA,
    gl.ONE_MINUS_SRC_ALPHA,
    gl.SRC_ALPHA,
    gl.ONE_MINUS_SRC_ALPHA,
  ]);
  output.finish();
  assert.deepEqual(buffers.get(null), [
    [0, 0, 0, 0.1],
    [0, 0.5 * bodyOpacity, bodyOpacity, bodyOpacity],
    [0, 0, 0, 0],
  ]);
  assert.deepEqual(buffers.get(scene)[2], [1, 1, 1, 1]);
  assert.deepEqual(buffers.get(scene)[1], [0, 0.5, 1, 1]);
  assert.equal(renderer.renderTextElement({}, {}, undefined, scene), true);
  assert.deepEqual(buffers.get(scene)[2], [0.2, 0.3, 0.4, 1]);
  assert.equal(bound, scene, "restore source FBO after mirroring text");
  output.finish();
  assert.deepEqual(buffers.get(null)[2], [0.2, 0.3, 0.4, 1]);
  output.dispose();
  assert.equal(renderer.renderGlassShadowPass, originalShadow);
  assert.equal(renderer.renderTextElement, originalText);
  assert.equal(deleted, 2);
});
