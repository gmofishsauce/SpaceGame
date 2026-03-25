import * as THREE from 'three';
import { OrbitControls } from 'three/addons/controls/OrbitControls.js';
import { CSS2DRenderer, CSS2DObject } from 'three/addons/renderers/CSS2DRenderer.js';
import { STAR_DATA } from './stardata.js';

const CAMERA_DISTANCE  = 35;    // light-years; > max dataset dist (~22.7 LY)
const STAR_RADIUS      = 0.15;  // sphere radius in LY
const STAR_COLOR       = 0xffffff;
const DASH_SIZE        = 0.4;   // LY
const GAP_SIZE         = 0.25;  // LY
const LINE_COLOR       = 0x66aaff;
const RAYCAST_THRESHOLD = 0.3;  // LY

// --- Scene ---
const scene = new THREE.Scene();
scene.background = new THREE.Color(0x000000);

// --- Camera ---
const camera = new THREE.PerspectiveCamera(
  60,
  window.innerWidth / window.innerHeight,
  0.01,
  500
);
camera.position.set(CAMERA_DISTANCE, CAMERA_DISTANCE, CAMERA_DISTANCE);
camera.lookAt(0, 0, 0);

// --- WebGL Renderer ---
const renderer = new THREE.WebGLRenderer({ antialias: true });
renderer.setPixelRatio(window.devicePixelRatio);
renderer.setSize(window.innerWidth, window.innerHeight);
document.body.appendChild(renderer.domElement);

// --- CSS2D Renderer (for labels) ---
const css2DRenderer = new CSS2DRenderer();
css2DRenderer.setSize(window.innerWidth, window.innerHeight);
css2DRenderer.domElement.style.position = 'absolute';
css2DRenderer.domElement.style.top = '0';
css2DRenderer.domElement.style.pointerEvents = 'none';
document.body.appendChild(css2DRenderer.domElement);

// --- Orbit Controls ---
const controls = new OrbitControls(camera, renderer.domElement);
controls.target.set(0, 0, 0);
controls.enableDamping = false;
controls.addEventListener('change', requestRender);

// --- Star Markers ---
const starGeometry = new THREE.SphereGeometry(STAR_RADIUS, 8, 6);
const starMaterial = new THREE.MeshBasicMaterial({ color: STAR_COLOR });

const starMeshes = [];  // non-Sol meshes only, for raycasting

for (let i = 0; i < STAR_DATA.length; i++) {
  const entry = STAR_DATA[i];
  const mesh = new THREE.Mesh(starGeometry, starMaterial);
  mesh.position.set(entry.x, entry.y, entry.z);
  mesh.userData.starIndex = i;
  scene.add(mesh);

  if (entry.isSol) {
    const div = document.createElement('div');
    div.className = 'star-label';
    div.textContent = 'Sol';
    const label = new CSS2DObject(div);
    label.position.set(0, 0, 0);
    mesh.add(label);
  } else {
    starMeshes.push(mesh);
  }
}

// --- Raycaster / Mouseover State ---
const raycaster = new THREE.Raycaster();
raycaster.params.Mesh = { threshold: RAYCAST_THRESHOLD };
const mouse = new THREE.Vector2();

let currentHoveredIndex = -1;
let hoverLabel = null;
let hoverLines = [];

function makeDashedLine(a, b) {
  const geometry = new THREE.BufferGeometry().setFromPoints([a, b]);
  const material = new THREE.LineDashedMaterial({
    color: LINE_COLOR,
    dashSize: DASH_SIZE,
    gapSize: GAP_SIZE,
  });
  const line = new THREE.Line(geometry, material);
  line.computeLineDistances();
  return line;
}

function showHoverElements(starIndex) {
  const star = STAR_DATA[starIndex];

  const div = document.createElement('div');
  div.className = 'star-label';
  div.textContent = star.displayName;
  hoverLabel = new CSS2DObject(div);
  hoverLabel.position.set(star.x, star.y, star.z);
  scene.add(hoverLabel);

  const foot = new THREE.Vector3(star.x, 0, star.z);
  hoverLines = [
    makeDashedLine(foot, new THREE.Vector3(star.x, 0, 0)),
    makeDashedLine(foot, new THREE.Vector3(0, 0, star.z)),
    makeDashedLine(foot, new THREE.Vector3(star.x, star.y, star.z)),
  ];
  for (const line of hoverLines) {
    scene.add(line);
  }
}

function clearHoverElements() {
  if (hoverLabel !== null) {
    scene.remove(hoverLabel);
    hoverLabel.element.remove();
    hoverLabel = null;
  }
  for (const line of hoverLines) {
    scene.remove(line);
    line.geometry.dispose();
    line.material.dispose();
  }
  hoverLines = [];
}

window.addEventListener('mousemove', (event) => {
  mouse.x =  (event.clientX / window.innerWidth)  * 2 - 1;
  mouse.y = -(event.clientY / window.innerHeight) * 2 + 1;

  raycaster.setFromCamera(mouse, camera);
  const intersects = raycaster.intersectObjects(starMeshes);

  if (intersects.length > 0) {
    const newIndex = intersects[0].object.userData.starIndex;
    if (newIndex !== currentHoveredIndex) {
      clearHoverElements();
      showHoverElements(newIndex);
      currentHoveredIndex = newIndex;
    }
  } else {
    if (currentHoveredIndex !== -1) {
      clearHoverElements();
      currentHoveredIndex = -1;
    }
  }

  requestRender();
});

// --- Demand Rendering ---
let renderPending = false;

function requestRender() {
  if (!renderPending) {
    renderPending = true;
    requestAnimationFrame(doRender);
  }
}

function doRender() {
  renderPending = false;
  renderer.render(scene, camera);
  css2DRenderer.render(scene, camera);
}

window.addEventListener('resize', () => {
  camera.aspect = window.innerWidth / window.innerHeight;
  camera.updateProjectionMatrix();
  renderer.setSize(window.innerWidth, window.innerHeight);
  css2DRenderer.setSize(window.innerWidth, window.innerHeight);
  requestRender();
});

requestRender();
