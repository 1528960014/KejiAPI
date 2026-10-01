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
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

const LOGO = {
  op: 'https://cos.lingkeai.vip/op.svg',
  claude: 'https://cos.lingkeai.vip/claude.svg',
  gem: 'https://cos.lingkeai.vip/gem.svg',
  deepseek: 'https://cos.lingkeai.vip/deepseek.svg',
  glm: 'https://cos.lingkeai.vip/glm.svg',
  qwen: 'https://cos.lingkeai.vip/qwen.svg',
  kimi: 'https://cos.lingkeai.vip/kimi.svg',
  minimax: 'https://cos.lingkeai.vip/minimax.svg',
  doubao: 'https://cos.lingkeai.vip/doubao.svg',
  kling: 'https://cos.lingkeai.vip/kling.svg',
  step: 'https://cos.lingkeai.vip/step.svg',
  mimo: 'https://cos.lingkeai.vip/mimo_bai.svg',
  banana: 'https://cos.lingkeai.vip/banana.svg',
}

interface FloatingChip {
  name: string
  logo: string
  top: number
  left: number
  size: number
  delay: number
}

const CHIPS: FloatingChip[] = [
  { name: 'TT Image 2.5', logo: LOGO.op, top: 9, left: 34, size: 40, delay: 0 },
  { name: 'SD 2.5 全能', logo: LOGO.doubao, top: 14, left: 41, size: 34, delay: 1.2 },
  { name: '千问 3.8 Max', logo: LOGO.qwen, top: 11, left: 30, size: 38, delay: 2.4 },
  { name: 'SD 2.0 全能', logo: LOGO.banana, top: 8, left: 66, size: 36, delay: 0.8 },
  { name: '万相3.0 文生', logo: LOGO.qwen, top: 13, left: 74, size: 34, delay: 1.9 },
  { name: 'SN-5.5', logo: LOGO.op, top: 17, left: 79, size: 30, delay: 3.1 },
  { name: 'GK-4.6', logo: LOGO.op, top: 22, left: 84, size: 32, delay: 0.4 },
  { name: 'TT-6 astra', logo: LOGO.op, top: 30, left: 24, size: 44, delay: 1.5 },
  { name: '千问 3.8 Ma', logo: LOGO.qwen, top: 27, left: 31, size: 32, delay: 2.8 },
  { name: 'SD 2.5 满血版', logo: LOGO.doubao, top: 33, left: 29, size: 36, delay: 0.6 },
  { name: '海螺 H3 Max', logo: LOGO.minimax, top: 40, left: 26, size: 34, delay: 2.1 },
  { name: '万相3.0 全能', logo: LOGO.qwen, top: 46, left: 24, size: 34, delay: 1.1 },
  { name: '千问 3.8 Flash', logo: LOGO.qwen, top: 53, left: 26, size: 32, delay: 3.4 },
  { name: 'Omni Flash', logo: LOGO.doubao, top: 60, left: 25, size: 30, delay: 0.9 },
  { name: 'DS-V4-1-Flash', logo: LOGO.deepseek, top: 66, left: 27, size: 32, delay: 2.6 },
  { name: 'GLM-5.3 Flash', logo: LOGO.glm, top: 72, left: 26, size: 32, delay: 1.7 },
  { name: 'SD 2.0 首图', logo: LOGO.banana, top: 78, left: 29, size: 32, delay: 0.2 },
  { name: 'GK-4.7', logo: LOGO.op, top: 84, left: 33, size: 30, delay: 3.8 },
  { name: 'TT-6 luna', logo: LOGO.op, top: 86, left: 40, size: 32, delay: 1.4 },
  { name: 'GK Image 2.0', logo: LOGO.op, top: 90, left: 36, size: 30, delay: 2.2 },
  { name: 'SD 2.5 文生', logo: LOGO.doubao, top: 88, left: 56, size: 32, delay: 0.5 },
  { name: 'GLM-5.3 Flash', logo: LOGO.glm, top: 91, left: 60, size: 30, delay: 3.0 },
  { name: 'MIMO', logo: LOGO.mimo, top: 29, left: 73, size: 34, delay: 2.0 },
  { name: 'mimo-v2.6', logo: LOGO.mimo, top: 35, left: 76, size: 30, delay: 0.7 },
  { name: 'SD 2.0 满血版', logo: LOGO.banana, top: 42, left: 77, size: 34, delay: 3.6 },
  { name: 'GLM-5.3', logo: LOGO.glm, top: 48, left: 76, size: 32, delay: 1.3 },
  { name: 'Omni 1.1', logo: LOGO.doubao, top: 55, left: 76, size: 30, delay: 2.9 },
  { name: 'DS-V4-Flash', logo: LOGO.deepseek, top: 61, left: 75, size: 32, delay: 0.3 },
  { name: 'GEM 3.7 flash', logo: LOGO.gem, top: 67, left: 74, size: 34, delay: 3.2 },
  { name: '万相 3.0', logo: LOGO.qwen, top: 73, left: 75, size: 32, delay: 1.8 },
  { name: 'OP-5.5', logo: LOGO.op, top: 79, left: 76, size: 30, delay: 2.5 },
  { name: '海螺 H3 Max', logo: LOGO.minimax, top: 84, left: 74, size: 30, delay: 0.1 },
  { name: 'GEM 3.6 flash', logo: LOGO.gem, top: 88, left: 70, size: 32, delay: 3.5 },
  { name: '千问 3.8 Flash', logo: LOGO.qwen, top: 91, left: 64, size: 30, delay: 1.6 },
  { name: 'Kimi K3', logo: LOGO.kimi, top: 20, left: 38, size: 30, delay: 2.7 },
  { name: 'Kling 2.5', logo: LOGO.kling, top: 24, left: 68, size: 30, delay: 3.9 },
  { name: 'Step 2.5', logo: LOGO.step, top: 82, left: 50, size: 30, delay: 2.3 },
]

function HighlightCard({
  side,
  logo,
  glow,
  titleKey,
  descKey,
}: {
  side: 'left' | 'right'
  logo: string
  glow: 'cyan' | 'violet'
  titleKey: string
  descKey: string
}) {
  const { t } = useTranslation()
  return (
    <div
      className={cn(
        'pointer-events-none absolute top-[36%] hidden w-44 lg:block',
        side === 'left' ? 'left-[7%]' : 'right-[7%]'
      )}
    >
      <div
        className={cn(
          'mx-auto flex size-14 items-center justify-center rounded-2xl border bg-[#0b1018]/90 p-2.5 shadow-2xl backdrop-blur',
          glow === 'cyan'
            ? 'border-cyan-400/50 shadow-[0_0_28px_rgba(34,211,238,0.35)]'
            : 'border-violet-400/50 shadow-[0_0_28px_rgba(167,139,250,0.35)]'
        )}
      >
        <img src={logo} alt='' className='size-full object-contain' />
      </div>
      <div
        className={cn(
          'mt-3 text-sm font-bold',
          glow === 'cyan' ? 'text-cyan-300' : 'text-violet-300'
        )}
      >
        {t(titleKey)}
      </div>
      <p className='mt-1.5 text-[11px] leading-relaxed text-slate-400/90'>
        {t(descKey)}
      </p>
    </div>
  )
}

/**
 * Scattered floating model-logo constellation behind the hero copy,
 * plus two highlighted model cards on the left / right edges.
 */
export function FloatingModels() {
  return (
    <div aria-hidden className='absolute inset-0 overflow-hidden'>
      <style>{`
        @keyframes floaty {
          0%, 100% { transform: translateY(0px); }
          50% { transform: translateY(-9px); }
        }
      `}</style>
      {/* Orbital arcs */}
      <svg
        className='absolute inset-x-0 top-[30%] h-[46%] w-full opacity-40'
        viewBox='0 0 1440 400'
        fill='none'
        preserveAspectRatio='none'
      >
        <path
          d='M-60 260 C 320 60, 1120 60, 1500 260'
          stroke='rgba(45, 212, 191, 0.28)'
          strokeWidth='1.2'
        />
        <path
          d='M-60 330 C 420 130, 1020 130, 1500 330'
          stroke='rgba(56, 189, 248, 0.18)'
          strokeWidth='1'
        />
      </svg>

      {CHIPS.map((chip, i) => (
        <div
          key={`${chip.name}-${i}`}
          className='absolute hidden flex-col items-center md:flex'
          style={{
            top: `${chip.top}%`,
            left: `${chip.left}%`,
            animation: `floaty ${6 + (i % 5)}s ease-in-out ${chip.delay}s infinite`,
            opacity: 0.75,
          }}
        >
          <div
            className='flex items-center justify-center rounded-xl border border-white/[0.07] bg-[#0b1018]/80 p-1 shadow-lg backdrop-blur-sm'
            style={{ width: chip.size, height: chip.size }}
          >
            <img
              src={chip.logo}
              alt=''
              loading='lazy'
              className='size-full object-contain'
            />
          </div>
          <span className='mt-1 max-w-[76px] truncate text-[9px] text-slate-500'>
            {chip.name}
          </span>
        </div>
      ))}

      <HighlightCard
        side='left'
        logo={LOGO.op}
        glow='cyan'
        titleKey='TT-6 astra'
        descKey='TT-6 astra is the next-gen flagship: reasoning, code and multimodal understanding upgraded, with ultra-long context support'
      />
      <HighlightCard
        side='right'
        logo={LOGO.qwen}
        glow='violet'
        titleKey='Wan 3.0 Reference'
        descKey='Wan 3.0 reference-to-video: direct text output, or lock characters with up to 10 reference images, plus uploads'
      />
    </div>
  )
}