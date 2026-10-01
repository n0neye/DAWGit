// A 3D model viewer on a canvas (three.js, loaded when first needed): the
// model framed by the camera, which the mouse turns (left), moves (right)
// and zooms (wheel). Two viewers can share a camera (onCamera/setCamera).

export type ModelStats = { triangles: number; size: [number, number, number] };

export type Viewer = {
  stats: ModelStats;
  setCamera: (state: number[]) => void;
  onCamera: (fn: (state: number[]) => void) => void;
  dispose: () => void;
};

export const modelExts = [".glb", ".gltf", ".fbx", ".obj", ".stl", ".ply"];

// url: the model; resolve: other files it names (a glTF's .bin and
// textures), by their path relative to the model.
export async function createViewer(canvas: HTMLCanvasElement, url: string, ext: string,
  resolve: (relative: string) => string): Promise<Viewer> {
  const THREE = await import("three");
  const { OrbitControls } = await import("three/examples/jsm/controls/OrbitControls.js");

  const manager = new THREE.LoadingManager();
  manager.setURLModifier((u) => (u === url || u.startsWith("blob:") || u.startsWith("data:") || u.includes("dawgit-file?")
    ? u : resolve(decodeURIComponent(u.replace(/^\.?\//, "")))));
  // Textures load after the model: those still without an image once all
  // are done weren't found.
  let loaded: any = null;
  let settled = false;
  manager.onLoad = () => {
    settled = true;
    if (loaded) showable(THREE, loaded, true);
  };
  const object = await load(THREE, manager, url, ext);
  loaded = object;
  showable(THREE, object, settled);

  const renderer = new THREE.WebGLRenderer({ canvas, antialias: true, alpha: true });
  renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
  const scene = new THREE.Scene();
  scene.add(new THREE.HemisphereLight(0xffffff, 0x444450, 2.2));
  const sun = new THREE.DirectionalLight(0xffffff, 2);
  sun.position.set(3, 5, 4);
  scene.add(sun);
  scene.add(object);

  // Frame it: centered, the camera back far enough to see it whole.
  const box = new THREE.Box3().setFromObject(object);
  const size = box.getSize(new THREE.Vector3());
  const center = box.getCenter(new THREE.Vector3());
  const radius = Math.max(size.length() / 2, 1e-3);
  const grid = new THREE.GridHelper(radius * 4, 20, 0x555a66, 0x34373e);
  grid.position.set(center.x, box.min.y, center.z);
  scene.add(grid);
  const camera = new THREE.PerspectiveCamera(40, 1, radius / 100, radius * 100);
  camera.position.copy(center).add(new THREE.Vector3(1, 0.7, 1.2).normalize().multiplyScalar(radius * 2.6));
  const controls = new OrbitControls(camera, canvas);
  controls.target.copy(center);
  controls.enableDamping = true;
  controls.update();

  let triangles = 0;
  object.traverse((o: any) => {
    const g = o.geometry;
    if (!o.isMesh || !g) return;
    triangles += (g.index ? g.index.count : g.attributes.position?.count ?? 0) / 3;
  });

  const listeners: ((s: number[]) => void)[] = [];
  let applying = false;
  controls.addEventListener("change", () => {
    if (applying) return;
    const s = [...camera.position.toArray(), ...controls.target.toArray()];
    for (const fn of listeners) fn(s);
  });

  const resize = () => {
    const w = canvas.clientWidth, h = canvas.clientHeight;
    if (!w || !h) return;
    renderer.setSize(w, h, false);
    camera.aspect = w / h;
    camera.updateProjectionMatrix();
  };
  const observer = new ResizeObserver(resize);
  observer.observe(canvas);
  resize();
  let raf = 0;
  const frame = () => {
    controls.update();
    renderer.render(scene, camera);
    raf = requestAnimationFrame(frame);
  };
  frame();

  return {
    stats: { triangles: Math.round(triangles), size: [size.x, size.y, size.z] },
    setCamera(s) {
      applying = true;
      camera.position.fromArray(s, 0);
      controls.target.fromArray(s, 3);
      controls.update();
      applying = false;
    },
    onCamera(fn) { listeners.push(fn); },
    dispose() {
      cancelAnimationFrame(raf);
      observer.disconnect();
      controls.dispose();
      scene.traverse((o: any) => {
        o.geometry?.dispose?.();
        for (const m of [o.material].flat()) m?.dispose?.();
      });
      renderer.dispose();
    },
  };
}

// showable makes every surface visible: both sides drawn (cards of grass,
// scans open at the back), nothing fully transparent, and textures that
// aren't in the project replaced by plain gray.
function showable(THREE: typeof import("three"), object: any, settled: boolean) {
  object.traverse((o: any) => {
    if (!o.isMesh) return;
    for (const m of [o.material].flat()) {
      if (!m) continue;
      m.side = THREE.DoubleSide;
      if (m.opacity !== undefined && m.opacity < 0.05) {
        m.opacity = 1;
        m.transparent = false;
      }
      for (const key of ["map", "alphaMap", "normalMap", "bumpMap", "specularMap", "emissiveMap"]) {
        const t = m[key];
        if (settled && t && !t.image) {
          m[key] = null;
          if (key === "map" && m.color) m.color.set(0xb8bcc6);
          if (key === "alphaMap") m.transparent = false;
        }
      }
      m.needsUpdate = true;
    }
  });
}

async function load(THREE: typeof import("three"), manager: any, url: string, ext: string): Promise<any> {
  const gray = () => new THREE.MeshStandardMaterial({ color: 0xb8bcc6, roughness: 0.6, metalness: 0.05 });
  switch (ext) {
    case ".glb":
    case ".gltf": {
      const { GLTFLoader } = await import("three/examples/jsm/loaders/GLTFLoader.js");
      return (await new GLTFLoader(manager).loadAsync(url)).scene;
    }
    case ".fbx": {
      const { FBXLoader } = await import("three/examples/jsm/loaders/FBXLoader.js");
      return await new FBXLoader(manager).loadAsync(url);
    }
    case ".obj": {
      const { OBJLoader } = await import("three/examples/jsm/loaders/OBJLoader.js");
      const obj = await new OBJLoader(manager).loadAsync(url);
      obj.traverse((o: any) => { if (o.isMesh) o.material = gray(); });
      return obj;
    }
    case ".stl":
    case ".ply": {
      const mod = ext === ".stl"
        ? await import("three/examples/jsm/loaders/STLLoader.js")
        : await import("three/examples/jsm/loaders/PLYLoader.js");
      const Loader = "STLLoader" in mod ? mod.STLLoader : (mod as any).PLYLoader;
      const geometry = await new Loader(manager).loadAsync(url);
      geometry.computeVertexNormals();
      const material = geometry.attributes.color
        ? new THREE.MeshStandardMaterial({ vertexColors: true, roughness: 0.6 }) : gray();
      return new THREE.Mesh(geometry, material);
    }
  }
  throw new Error("not a 3D model this can show");
}
