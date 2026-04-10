<template>
  <div :class="['letter-glitch', props.class]" :style="containerStyle">
    <canvas ref="canvasRef" :style="canvasStyle" />
    <div v-if="outerVignette" class="letter-glitch__vignette-outer" />
    <div v-if="centerVignette" class="letter-glitch__vignette-center" />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch } from 'vue'

interface Props {
  glitchColors?: string[]
  class?: string
  glitchSpeed?: number
  centerVignette?: boolean
  outerVignette?: boolean
  smooth?: boolean
  characters?: string
}

const props = withDefaults(defineProps<Props>(), {
  glitchColors: () => ['#2b4539', '#61dca3', '#61b3dc'],
  class: '',
  glitchSpeed: 50,
  centerVignette: false,
  outerVignette: true,
  smooth: true,
  characters: 'ABCDEFGHIJKLMNOPQRSTUVWXYZ!@#$&*()-_+=/[]{};:<>.,0123456789'
})

const canvasRef = ref<HTMLCanvasElement | null>(null)

let animationId: number | null = null
let ctx: CanvasRenderingContext2D | null = null
let letters: Array<{
  char: string
  color: string
  startColor: string
  targetColor: string
  colorProgress: number
}> = []
let grid = { columns: 0, rows: 0 }
let lastGlitchTime = Date.now()
let prefersReducedMotion = false

const fontSize = 16
const charWidth = 10
const charHeight = 20

const lettersAndSymbols = Array.from(props.characters)

const containerStyle = {
  position: 'relative' as const,
  width: '100%',
  height: '100%',
  backgroundColor: '#000000',
  overflow: 'hidden'
}

const canvasStyle = {
  display: 'block',
  width: '100%',
  height: '100%'
}

function getRandomChar(): string {
  return lettersAndSymbols[Math.floor(Math.random() * lettersAndSymbols.length)]
}

function getRandomColor(): string {
  return props.glitchColors[Math.floor(Math.random() * props.glitchColors.length)]
}

function hexToRgb(hex: string): { r: number; g: number; b: number } | null {
  const shorthandRegex = /^#?([a-f\d])([a-f\d])([a-f\d])$/i
  hex = hex.replace(shorthandRegex, (_m, r, g, b) => r + r + g + g + b + b)
  const result = /^#?([a-f\d]{2})([a-f\d]{2})([a-f\d]{2})$/i.exec(hex)
  return result
    ? {
        r: parseInt(result[1], 16),
        g: parseInt(result[2], 16),
        b: parseInt(result[3], 16)
      }
    : null
}

function interpolateColor(
  start: { r: number; g: number; b: number },
  end: { r: number; g: number; b: number },
  factor: number
): string {
  const r = Math.round(start.r + (end.r - start.r) * factor)
  const g = Math.round(start.g + (end.g - start.g) * factor)
  const b = Math.round(start.b + (end.b - start.b) * factor)
  return `rgb(${r}, ${g}, ${b})`
}

function calculateGrid(width: number, height: number) {
  return {
    columns: Math.ceil(width / charWidth),
    rows: Math.ceil(height / charHeight)
  }
}

function initializeLetters(columns: number, rows: number) {
  grid = { columns, rows }
  const total = columns * rows
  letters = Array.from({ length: total }, () => {
    const color = getRandomColor()
    return {
      char: getRandomChar(),
      color,
      startColor: color,
      targetColor: getRandomColor(),
      colorProgress: 1
    }
  })
}

function resizeCanvas() {
  const canvas = canvasRef.value
  if (!canvas) return
  const parent = canvas.parentElement
  if (!parent) return

  const dpr = window.devicePixelRatio || 1
  const rect = parent.getBoundingClientRect()

  canvas.width = rect.width * dpr
  canvas.height = rect.height * dpr
  canvas.style.width = `${rect.width}px`
  canvas.style.height = `${rect.height}px`

  if (ctx) {
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
  }

  const { columns, rows } = calculateGrid(rect.width, rect.height)
  initializeLetters(columns, rows)
  drawLetters()
}

function drawLetters() {
  if (!ctx || letters.length === 0 || !canvasRef.value) return
  const { width, height } = canvasRef.value.getBoundingClientRect()
  ctx.clearRect(0, 0, width, height)
  ctx.font = `${fontSize}px monospace`
  ctx.textBaseline = 'top'

  for (let i = 0; i < letters.length; i++) {
    const letter = letters[i]
    const x = (i % grid.columns) * charWidth
    const y = Math.floor(i / grid.columns) * charHeight
    ctx.fillStyle = letter.color
    ctx.fillText(letter.char, x, y)
  }
}

function updateLetters() {
  if (!letters || letters.length === 0) return
  const updateCount = Math.max(1, Math.floor(letters.length * 0.05))

  for (let i = 0; i < updateCount; i++) {
    const index = Math.floor(Math.random() * letters.length)
    if (!letters[index]) continue

    letters[index].char = getRandomChar()
    letters[index].targetColor = getRandomColor()

    if (!props.smooth) {
      letters[index].color = letters[index].targetColor
      letters[index].startColor = letters[index].targetColor
      letters[index].colorProgress = 1
    } else {
      letters[index].startColor = letters[index].targetColor
      letters[index].targetColor = getRandomColor()
      letters[index].colorProgress = 0
    }
  }
}

function handleSmoothTransitions() {
  let needsRedraw = false
  for (const letter of letters) {
    if (letter.colorProgress < 1) {
      letter.colorProgress += 0.05
      if (letter.colorProgress > 1) letter.colorProgress = 1

      const startRgb = hexToRgb(letter.startColor)
      const endRgb = hexToRgb(letter.targetColor)
      if (startRgb && endRgb) {
        letter.color = interpolateColor(startRgb, endRgb, letter.colorProgress)
        needsRedraw = true
      }
    }
  }
  if (needsRedraw) {
    drawLetters()
  }
}

function animate() {
  if (prefersReducedMotion) {
    // Draw once and stop when reduced motion is preferred
    drawLetters()
    return
  }

  const now = Date.now()
  if (now - lastGlitchTime >= props.glitchSpeed) {
    updateLetters()
    drawLetters()
    lastGlitchTime = now
  }

  if (props.smooth) {
    handleSmoothTransitions()
  }

  animationId = requestAnimationFrame(animate)
}

function start() {
  const canvas = canvasRef.value
  if (!canvas) return
  ctx = canvas.getContext('2d')
  resizeCanvas()
  animate()
}

function stop() {
  if (animationId !== null) {
    cancelAnimationFrame(animationId)
    animationId = null
  }
}

let resizeTimeout: ReturnType<typeof setTimeout> | undefined

function handleResize() {
  clearTimeout(resizeTimeout)
  resizeTimeout = setTimeout(() => {
    stop()
    resizeCanvas()
    animate()
  }, 100)
}

function handleVisibilityChange() {
  if (document.hidden) {
    stop()
  } else if (!prefersReducedMotion) {
    animate()
  }
}

onMounted(() => {
  const mq = window.matchMedia('(prefers-reduced-motion: reduce)')
  prefersReducedMotion = mq.matches
  mq.addEventListener('change', (e) => {
    prefersReducedMotion = e.matches
    if (e.matches) {
      stop()
      drawLetters()
    } else {
      animate()
    }
  })

  start()
  window.addEventListener('resize', handleResize)
  document.addEventListener('visibilitychange', handleVisibilityChange)
})

onUnmounted(() => {
  stop()
  window.removeEventListener('resize', handleResize)
  document.removeEventListener('visibilitychange', handleVisibilityChange)
  clearTimeout(resizeTimeout)
})

watch(
  () => [props.glitchSpeed, props.smooth],
  () => {
    stop()
    start()
  }
)
</script>

<style scoped>
.letter-glitch__vignette-outer {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
  background: radial-gradient(circle, rgba(0, 0, 0, 0) 60%, rgba(0, 0, 0, 1) 100%);
}

.letter-glitch__vignette-center {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
  background: radial-gradient(circle, rgba(0, 0, 0, 0.8) 0%, rgba(0, 0, 0, 0) 60%);
}
</style>
