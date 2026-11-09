import { STRIDE, type Buf, type Particles } from "./particles";
import { hash, mkRng } from "./rand";
import { kelvin } from "./particles";

// Slot data (centre, scale, orientation, alpha) lives in a float texture, one row per galaxy or
// cluster, so the number of sub-themes is not capped by the vertex uniform budget.
const VCOMMON = `#version 300 es
precision highp float;
layout(location=0) in vec4 aA; layout(location=1) in vec4 aB; layout(location=2) in vec4 aC; layout(location=3) in float aSize;
uniform highp sampler2D uSlots;
uniform vec3 uCam; uniform vec2 uView; uniform float uT, uDpr;
vec2 worldPos(out vec4 S) {
  int s = int(aB.w + .5);
  S = texelFetch(uSlots, ivec2(0, s), 0); vec4 O = texelFetch(uSlots, ivec2(1, s), 0); vec4 P = texelFetch(uSlots, ivec2(2, s), 0);
  float fr = aA.w + aB.x * uT;
  float r = aA.x * (1. - aA.z * .5 * (1. - cos(O.w * fr))) + aB.y;
  float psi = aA.y + fr;
  float x = r * cos(psi), y = r * sin(psi);
  float x1 = x * O.x - y * O.y, y1 = (x * O.y + y * O.x) * O.z + aB.z * P.w;
  return S.xy + S.z * vec2(x1 * P.x - y1 * P.y, x1 * P.y + y1 * P.x);
}
vec2 clip(vec2 w) { vec2 s = (w - uCam.xy) * uCam.z; return vec2(s.x / (uView.x * .5), -s.y / (uView.y * .5)); }
`;

const STARS_VS = VCOMMON + `out vec4 vC;
void main() {
  vec4 S; vec2 w = worldPos(S); gl_Position = vec4(clip(w), 0, 1);
  float rp = S.z * uCam.z; float dens = clamp(pow(rp / 420., 1.25), .1, 1.);
  gl_PointSize = max(1., aSize * uDpr * clamp(pow(rp / 380., .22), .75, 2.4));
  vC = vec4(aC.rgb, aC.a * S.w * dens);
}`;
const STARS_FS = `#version 300 es
precision mediump float; in vec4 vC; out vec4 o;
void main() { vec2 d = gl_PointCoord * 2. - 1.; float a = exp(-dot(d, d) * 3.2); o = vec4(vC.rgb * vC.a * a, vC.a * a); }`;

const QUADS_VS = VCOMMON + `layout(location=4) in vec2 aQ; uniform vec2 uFade; uniform float uDustQ; out vec4 vC; out vec2 vQ;
void main() {
  vec4 S; vec2 w = worldPos(S); float px = aSize * S.z * uCam.z;
  float f = (1. - smoothstep(uFade.x, uFade.y, px)) * smoothstep(.6, 2.5, px) * mix(1., smoothstep(3., 14., px), uDustQ);
  vC = vec4(aC.rgb, aC.a * S.w * f); vQ = aQ;
  if (vC.a < .002) { gl_Position = vec4(2, 2, 2, 1); return; }
  gl_Position = vec4(clip(w) + aQ * px / (uView * .5) * vec2(1, -1), 0, 1);
}`;
const QUADS_FS = `#version 300 es
precision mediump float; in vec4 vC; in vec2 vQ; uniform float uDust; out vec4 o;
void main() { float a = exp(-dot(vQ, vQ) * 4.5) * vC.a; o = uDust > .5 ? vec4(0, 0, 0, a) : vec4(vC.rgb * a, a); }`;

const TRI_VS = `#version 300 es
in vec2 p; out vec2 v; void main() { v = p * .5 + .5; gl_Position = vec4(p, 0, 1); }`;

// Nebula: fBm noise warped twice (Inigo Quilez's domain warping), a Milky-Way band and dark lanes.
const NEB_FS = `#version 300 es
precision highp float; uniform vec2 uRes; uniform vec3 uN; uniform float uT; out vec4 o;
float h(vec2 p) { p = fract(p * vec2(123.34, 456.21)); p += dot(p, p + 45.32); return fract(p.x * p.y); }
float n(vec2 p) { vec2 i = floor(p), f = fract(p); vec2 u = f * f * f * (f * (f * 6. - 15.) + 10.);
  return mix(mix(h(i), h(i + vec2(1, 0)), u.x), mix(h(i + vec2(0, 1)), h(i + 1.), u.x), u.y); }
float fbm(vec2 p) { float s = 0., a = .5; mat2 m = mat2(1.6, 1.2, -1.2, 1.6); for (int i = 0; i < 5; i++) { s += a * n(p); p = m * p; a *= .5; } return s; }
void main() {
  vec2 uv = (gl_FragCoord.xy - .5 * uRes) / uRes.y; vec2 p = uv * uN.z + uN.xy;
  vec2 q = vec2(fbm(p * 1.5), fbm(p * 1.5 + vec2(5.2, 1.3)));
  vec2 r = vec2(fbm(p * 1.5 + 3.2 * q + vec2(1.7, 9.2) + .01 * uT), fbm(p * 1.5 + 3.2 * q + vec2(8.3, 2.8) - .008 * uT));
  float f = fbm(p * 1.5 + 3. * r);
  float band = exp(-pow((uv.y + uv.x * .38 + .04) * 2., 2.));
  vec3 col = vec3(.010, .014, .032);
  col += mix(vec3(.09, .05, .20), vec3(.02, .13, .20), clamp(q.x * 1.5 - .25, 0., 1.)) * pow(f, 2.) * 1.15;
  col += vec3(.32, .07, .20) * pow(clamp(r.y * 1.25 - .32, 0., 1.), 3.) * (.35 + .9 * band);
  col += vec3(.20, .18, .26) * pow(f, 3.) * band * .9;
  float lanes = smoothstep(.42, .72, fbm(p * 3. + r * 2.2)); col *= 1. - .6 * lanes * band;
  col *= 1. - .3 * dot(uv, uv); o = vec4(col, 1);
}`;

const BLIT_FS = `#version 300 es
precision mediump float; in vec2 v; uniform sampler2D uTex; out vec4 o;
float h(vec2 p) { return fract(sin(dot(p, vec2(12.9898, 78.233))) * 43758.5453); }
void main() { o = vec4(texture(uTex, v).rgb + (h(gl_FragCoord.xy) - .5) / 255., 1); }`;

const BG_VS = `#version 300 es
precision highp float; layout(location=0) in vec4 a; layout(location=1) in vec4 c; uniform vec2 uView, uOff; uniform float uT, uDpr; out vec4 vC;
void main() {
  vec2 per = uView + 40.; vec2 s = mod(a.xy * per - uOff * a.z, per) - 20.;
  gl_Position = vec4(s.x / uView.x * 2. - 1., 1. - s.y / uView.y * 2., 0, 1);
  gl_PointSize = a.w * uDpr; float tw = c.a < 0. ? (.6 + .4 * sin(uT * 1.7 - c.a * 9.)) : 1.; vC = vec4(c.rgb, abs(c.a) * tw);
}`;
const BG_FS = `#version 300 es
precision mediump float; in vec4 vC; out vec4 o;
void main() { vec2 d = gl_PointCoord * 2. - 1.; float a = exp(-dot(d, d) * 3.5) * vC.a; o = vec4(vC.rgb * a, a); }`;

interface Prog { p: WebGLProgram; u: Record<string, WebGLUniformLocation> }
interface Vao { v: WebGLVertexArrayObject; n: number; buf: WebGLBuffer }

export interface Camera { x: number; y: number; k: number }

export class SkyGL {
  private gl: WebGL2RenderingContext;
  private progs: Record<string, Prog> = {};
  private tri!: WebGLVertexArrayObject;
  private quad!: WebGLBuffer;
  private bg!: Vao;
  private parts: Record<string, Vao> = {};
  private slotTex!: WebGLTexture;
  private slotData = new Float32Array(0);
  private slotRows = 0;
  private neb = { tex: null as WebGLTexture | null, fb: null as WebGLFramebuffer | null, w: 1, h: 1, key: "" };

  static create(canvas: HTMLCanvasElement): SkyGL | null {
    const gl = canvas.getContext("webgl2", { alpha: false, antialias: false, depth: false, stencil: false, powerPreference: "high-performance" });
    if (!gl) return null;
    try {
      return new SkyGL(gl);
    } catch (err) {
      console.error("sky: WebGL init failed", err);
      return null;
    }
  }

  // Browsers cap live WebGL contexts; switching views must give this one back.
  release() {
    this.gl.getExtension("WEBGL_lose_context")?.loseContext();
  }

  private constructor(gl: WebGL2RenderingContext) {
    this.gl = gl;
    this.progs.stars = this.compile(STARS_VS, STARS_FS);
    this.progs.quads = this.compile(QUADS_VS, QUADS_FS);
    this.progs.neb = this.compile(TRI_VS, NEB_FS);
    this.progs.blit = this.compile(TRI_VS, BLIT_FS);
    this.progs.bg = this.compile(BG_VS, BG_FS);

    const triBuf = gl.createBuffer()!;
    gl.bindBuffer(gl.ARRAY_BUFFER, triBuf);
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 3, -1, -1, 3]), gl.STATIC_DRAW);
    this.tri = gl.createVertexArray()!;
    gl.bindVertexArray(this.tri);
    gl.enableVertexAttribArray(0);
    gl.vertexAttribPointer(0, 2, gl.FLOAT, false, 0, 0);
    gl.bindVertexArray(null);

    this.quad = gl.createBuffer()!;
    gl.bindBuffer(gl.ARRAY_BUFFER, this.quad);
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 1, -1, 1, 1, -1, -1, 1, 1, -1, 1]), gl.STATIC_DRAW);

    this.bg = this.backgroundStars();
    this.slotTex = gl.createTexture()!;
    this.neb.tex = gl.createTexture();
    this.neb.fb = gl.createFramebuffer();
  }

  private compile(vs: string, fs: string): Prog {
    const gl = this.gl;
    const sh = (type: number, src: string) => {
      const s = gl.createShader(type)!;
      gl.shaderSource(s, src);
      gl.compileShader(s);
      if (!gl.getShaderParameter(s, gl.COMPILE_STATUS)) throw new Error(gl.getShaderInfoLog(s) ?? "shader");
      return s;
    };
    const p = gl.createProgram()!;
    gl.attachShader(p, sh(gl.VERTEX_SHADER, vs));
    gl.attachShader(p, sh(gl.FRAGMENT_SHADER, fs));
    gl.linkProgram(p);
    if (!gl.getProgramParameter(p, gl.LINK_STATUS)) throw new Error(gl.getProgramInfoLog(p) ?? "link");
    const u: Record<string, WebGLUniformLocation> = {};
    const n = gl.getProgramParameter(p, gl.ACTIVE_UNIFORMS);
    for (let i = 0; i < n; i++) {
      const a = gl.getActiveUniform(p, i)!;
      u[a.name.replace("[0]", "")] = gl.getUniformLocation(p, a.name)!;
    }
    return { p, u };
  }

  private backgroundStars(): Vao {
    const gl = this.gl;
    const r = mkRng(hash("background"));
    const n = 5200, d: number[] = [];
    for (let i = 0; i < n; i++) {
      const b = Math.pow(r(), 4), t = r(), band = r() < 0.45;
      let y = r();
      if (band) y = 0.5 + ((r() + r() + r()) / 3 - 0.5) * 0.5;
      const col = t < 0.18 ? kelvin(3600 + r() * 1500) : t < 0.4 ? kelvin(9000 + r() * 15000) : kelvin(5500 + r() * 1500);
      d.push(r(), y, 0.3 + r() * 0.7, 0.8 + b * 2.4, col[0], col[1], col[2], (0.25 + b * 0.75) * (r() < 0.08 ? -1 : 1));
    }
    const v = gl.createVertexArray()!;
    gl.bindVertexArray(v);
    const buf = gl.createBuffer()!;
    gl.bindBuffer(gl.ARRAY_BUFFER, buf);
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array(d), gl.STATIC_DRAW);
    gl.enableVertexAttribArray(0);
    gl.vertexAttribPointer(0, 4, gl.FLOAT, false, 32, 0);
    gl.enableVertexAttribArray(1);
    gl.vertexAttribPointer(1, 4, gl.FLOAT, false, 32, 16);
    gl.bindVertexArray(null);
    return { v, n, buf };
  }

  setParticles(P: Particles, slotCount: number) {
    const gl = this.gl;
    for (const old of Object.values(this.parts)) {
      gl.deleteBuffer(old.buf);
      gl.deleteVertexArray(old.v);
    }
    const make = (B: Buf, instanced: boolean): Vao => {
      const v = gl.createVertexArray()!;
      gl.bindVertexArray(v);
      const buf = gl.createBuffer()!;
      gl.bindBuffer(gl.ARRAY_BUFFER, buf);
      gl.bufferData(gl.ARRAY_BUFFER, new Float32Array(B.d), gl.STATIC_DRAW);
      const st = STRIDE * 4;
      for (const [l, s, o] of [[0, 4, 0], [1, 4, 16], [2, 4, 32], [3, 1, 48]]) {
        gl.enableVertexAttribArray(l);
        gl.vertexAttribPointer(l, s, gl.FLOAT, false, st, o);
        if (instanced) gl.vertexAttribDivisor(l, 1);
      }
      if (instanced) {
        gl.bindBuffer(gl.ARRAY_BUFFER, this.quad);
        gl.enableVertexAttribArray(4);
        gl.vertexAttribPointer(4, 2, gl.FLOAT, false, 0, 0);
      }
      gl.bindVertexArray(null);
      return { v, n: B.n, buf };
    };
    this.parts = { stars: make(P.stars, false), glow: make(P.glow, true), dust: make(P.dust, true), hii: make(P.hii, true) };
    this.slotRows = Math.max(1, slotCount);
    this.slotData = new Float32Array(this.slotRows * 12);
  }

  /** Row i: [cx, cy, scale, alpha], [cosθ, sinθ, tilt, arms], [cosφ, sinφ, 0, z-projection]. */
  get slots() {
    return this.slotData;
  }

  resize(w: number, h: number, dpr: number) {
    const gl = this.gl;
    this.neb.w = Math.max(1, (w * dpr * 0.5) | 0);
    this.neb.h = Math.max(1, (h * dpr * 0.5) | 0);
    gl.bindTexture(gl.TEXTURE_2D, this.neb.tex);
    gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, this.neb.w, this.neb.h, 0, gl.RGBA, gl.UNSIGNED_BYTE, null);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
    gl.bindFramebuffer(gl.FRAMEBUFFER, this.neb.fb);
    gl.framebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, this.neb.tex, 0);
    gl.bindFramebuffer(gl.FRAMEBUFFER, null);
    this.neb.key = "";
  }

  draw(cam: Camera, W: number, H: number, dpr: number, t: number, animate: boolean) {
    const gl = this.gl, k = cam.k, time = animate ? t : 0;
    const cw = gl.canvas.width, ch = gl.canvas.height;
    gl.viewport(0, 0, cw, ch);
    gl.disable(gl.BLEND);

    const zs = Math.exp(-0.1 * Math.log(k / 0.08)), nx = cam.x / 26000, ny = -cam.y / 26000;
    const key = [nx.toFixed(4), ny.toFixed(4), zs.toFixed(4), animate ? Math.floor(t * 6) : 0].join();
    if (key !== this.neb.key) {
      this.neb.key = key;
      gl.bindFramebuffer(gl.FRAMEBUFFER, this.neb.fb);
      gl.viewport(0, 0, this.neb.w, this.neb.h);
      const p = this.progs.neb;
      gl.useProgram(p.p);
      gl.uniform2f(p.u.uRes, this.neb.w, this.neb.h);
      gl.uniform3f(p.u.uN, nx, ny, zs);
      gl.uniform1f(p.u.uT, time);
      gl.bindVertexArray(this.tri);
      gl.drawArrays(gl.TRIANGLES, 0, 3);
      gl.bindFramebuffer(gl.FRAMEBUFFER, null);
      gl.viewport(0, 0, cw, ch);
    }
    {
      const p = this.progs.blit;
      gl.useProgram(p.p);
      gl.activeTexture(gl.TEXTURE0);
      gl.bindTexture(gl.TEXTURE_2D, this.neb.tex);
      gl.uniform1i(p.u.uTex, 0);
      gl.bindVertexArray(this.tri);
      gl.drawArrays(gl.TRIANGLES, 0, 3);
    }
    gl.enable(gl.BLEND);
    gl.blendFunc(gl.ONE, gl.ONE);
    {
      const p = this.progs.bg;
      gl.useProgram(p.p);
      gl.uniform2f(p.u.uView, W, H);
      gl.uniform2f(p.u.uOff, cam.x * 0.012, cam.y * 0.012);
      gl.uniform1f(p.u.uT, time);
      gl.uniform1f(p.u.uDpr, dpr);
      gl.bindVertexArray(this.bg.v);
      gl.drawArrays(gl.POINTS, 0, this.bg.n);
    }
    if (!this.parts.stars) return;

    gl.activeTexture(gl.TEXTURE1);
    gl.bindTexture(gl.TEXTURE_2D, this.slotTex);
    gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA32F, 3, this.slotRows, 0, gl.RGBA, gl.FLOAT, this.slotData);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST);

    const common = (p: Prog) => {
      gl.useProgram(p.p);
      gl.uniform1i(p.u.uSlots, 1);
      gl.uniform3f(p.u.uCam, cam.x, cam.y, k);
      gl.uniform2f(p.u.uView, W, H);
      gl.uniform1f(p.u.uT, time);
      if (p.u.uDpr) gl.uniform1f(p.u.uDpr, dpr);
    };
    const q = this.progs.quads;
    const quads = (name: string, fa: number, fb: number, dust: boolean) => {
      gl.uniform2f(q.u.uFade, fa, fb);
      gl.uniform1f(q.u.uDust, dust ? 1 : 0);
      gl.uniform1f(q.u.uDustQ, dust ? 1 : 0);
      gl.bindVertexArray(this.parts[name].v);
      gl.drawArraysInstanced(gl.TRIANGLES, 0, 6, this.parts[name].n);
    };
    common(q);
    quads("glow", 70, 240, false);
    common(this.progs.stars);
    gl.bindVertexArray(this.parts.stars.v);
    gl.drawArrays(gl.POINTS, 0, this.parts.stars.n);
    common(q);
    gl.blendFunc(gl.ZERO, gl.ONE_MINUS_SRC_ALPHA);
    quads("dust", 45, 160, true);
    gl.blendFunc(gl.ONE, gl.ONE);
    quads("hii", 14, 45, false);
    gl.bindVertexArray(null);
  }
}
