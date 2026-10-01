/*
Copyright (C) 2023-2026 1528960014

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@kejiapi.com
*/
import { useEffect, useRef } from 'react'

const STAR_COLORS = [
  '170, 225, 255',
  '150, 255, 214',
  '205, 185, 255',
  '255, 224, 178',
  '120, 240, 255',
]

interface Star {
  x: number
  y: number
  r: number
  baseAlpha: number
  phase: number
  speed: number
  color: string
}

/**
 * Self-drawn nebula starfield: a full-bleed canvas with softly
 * twinkling multi-color stars used as the hero backdrop.
 */
export function StarfieldCanvas() {
  const canvasRef = useRef<HTMLCanvasElement>(null)

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return

    let raf = 0
    let width = 0
    let height = 0
    let stars: Star[] = []
    const dpr = Math.min(window.devicePixelRatio || 1, 2)
    const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches

    const build = () => {
      width = canvas.clientWidth
      height = canvas.clientHeight
      canvas.width = Math.max(width * dpr, 1)
      canvas.height = Math.max(height * dpr, 1)
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
      const count = Math.min(Math.floor((width * height) / 7000), 260)
      stars = Array.from({ length: count }, () => ({
        x: Math.random() * width,
        y: Math.random() * height,
        r: Math.random() * 1.4 + 0.35,
        baseAlpha: Math.random() * 0.55 + 0.2,
        phase: Math.random() * Math.PI * 2,
        speed: Math.random() * 0.9 + 0.4,
        color: STAR_COLORS[Math.floor(Math.random() * STAR_COLORS.length)],
      }))
    }

    let t = 0
    const frame = () => {
      t += 0.016
      ctx.clearRect(0, 0, width, height)
      for (const s of stars) {
        const alpha = reduced
          ? s.baseAlpha
          : s.baseAlpha * (0.55 + 0.45 * Math.sin(t * s.speed + s.phase))
        ctx.beginPath()
        ctx.arc(s.x, s.y, s.r, 0, Math.PI * 2)
        ctx.fillStyle = `rgba(${s.color}, ${alpha.toFixed(3)})`
        ctx.fill()
      }
      raf = requestAnimationFrame(frame)
    }

    build()
    raf = requestAnimationFrame(frame)
    window.addEventListener('resize', build)
    return () => {
      cancelAnimationFrame(raf)
      window.removeEventListener('resize', build)
    }
  }, [])

  return (
    <canvas
      ref={canvasRef}
      aria-hidden
      className='absolute inset-0 size-full'
    />
  )
}