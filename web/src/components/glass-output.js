// The upstream canvas includes an opaque wallpaper/background. Mirror ONLY its
// existing shadow, material composite and foreground passes into a transparent
// target for DOM placement, including navigation text when requested.
// All shaders, uniforms and physics remain upstream.
export function createGlassOutput(renderer, { bodyOpacity = 1, mirrorText = false } = {}) {
  const gl = renderer.gl;
  const bind = gl.bindFramebuffer.bind(gl);
  const blend = gl.blendFunc.bind(gl);
  const blendSeparate = gl.blendFuncSeparate.bind(gl);
  let framebuffer = null,
    factors = [gl.ONE, gl.ZERO, gl.ONE, gl.ZERO];
  let mirroring = false, mirroredPass,
    target;
  gl.bindFramebuffer = (kind, value) => {
    framebuffer = value;
    bind(kind, value);
  };
  gl.blendFunc = (src, dst) => {
    factors = [src, dst, src, dst];
    // RGB is exactly the source pass. On a transparent target, accumulate
    // coverage with SrcOver instead of squaring shadow alpha against zero.
    if (mirroring)
      blendSeparate(src, dst, dst === gl.ONE ? gl.ZERO : gl.ONE, dst);
    else blend(src, dst);
  };
  gl.blendFuncSeparate = (...values) => {
    factors = values;
    if (mirroring && mirroredPass === "drawElFboComposite" && bodyOpacity < 1) {
      // The source glass includes a sampled opaque scene. DOM containers need
      // translucent body coverage, while shadow/highlight passes stay intact.
      gl.blendColor(0, 0, 0, bodyOpacity);
      blendSeparate(gl.CONSTANT_ALPHA, gl.ONE_MINUS_SRC_ALPHA,
        gl.CONSTANT_ALPHA, gl.ONE_MINUS_SRC_ALPHA);
    } else blendSeparate(...values);
  };
  const originals = new Map();
  for (const name of [
    "renderGlassShadowPass",
    "drawElFboComposite",
    "renderGlassPostPasses",
    ...(mirrorText ? ["renderTextElement"] : []),
  ]) {
    const original = renderer[name];
    originals.set(name, original);
    renderer[name] = function (...args) {
      const result = original.apply(this, args);
      if (!target) return result;
      const previous = framebuffer,
        previousFactors = factors.slice();
      bind(gl.FRAMEBUFFER, target.fb);
      // Start with the blend mode left by the source pass, then allow that
      // same method to select its own modes while replaying its draw calls.
      blendSeparate(
        previousFactors[0],
        previousFactors[1],
        previousFactors[1] === gl.ONE ? gl.ZERO : gl.ONE,
        previousFactors[3],
      );
      mirroring = true;
      mirroredPass = name;
      try {
        if (name === "renderTextElement") {
          // Upstream text binds its destination explicitly. Keep it in the
          // source scene for refraction AND mirror it for unselected labels.
          original.call(this, args[0], args[1], args[2], target.fb);
        } else original.apply(this, args);
      } finally {
        mirroring = false;
        mirroredPass = undefined;
        bind(gl.FRAMEBUFFER, previous);
        framebuffer = previous;
        blendSeparate(...previousFactors);
        factors = previousFactors;
      }
      return result;
    };
  }
  return {
    begin() {
      if (
        !target ||
        target.width !== renderer.fboW ||
        target.height !== renderer.fboH
      ) {
        if (target) {
          gl.deleteFramebuffer(target.fb);
          gl.deleteTexture(target.tex);
        }
        target = {
          ...renderer.createFBO(renderer.fboW, renderer.fboH),
          width: renderer.fboW,
          height: renderer.fboH,
        };
      }
      renderer.bindFBO(target.fb);
      gl.disable(gl.SCISSOR_TEST);
      gl.clearColor(0, 0, 0, 0);
      gl.clear(gl.COLOR_BUFFER_BIT);
    },
    finish() {
      renderer.bindFBO(null);
      renderer.drawCopy(target.tex);
    },
    dispose() {
      for (const [name, original] of originals) renderer[name] = original;
      gl.bindFramebuffer = bind;
      gl.blendFunc = blend;
      gl.blendFuncSeparate = blendSeparate;
      if (target) {
        gl.deleteFramebuffer(target.fb);
        gl.deleteTexture(target.tex);
      }
      target = undefined;
    },
  };
}
