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
import {
  AudioLines,
  Clapperboard,
  MessageCircle,
  Palette,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { AnimateInView } from '@/components/animate-in-view'

const capabilities = [
  {
    id: 'chat',
    title: 'Smart Chat',
    desc: 'Switch freely between mainstream models: multi-turn reasoning, code, writing and translation in one place.',
    icon: <MessageCircle className='size-5 text-cyan-300/90' strokeWidth={1.5} />,
    tint: 'border-cyan-400/20 bg-cyan-400/[0.07]',
  },
  {
    id: 'image',
    title: 'Image Creation',
    desc: 'Pick from mainstream image models; type a prompt and get results instantly, in photoreal, illustration or 3D styles.',
    icon: <Palette className='size-5 text-violet-300/90' strokeWidth={1.5} />,
    tint: 'border-violet-400/20 bg-violet-400/[0.07]',
  },
  {
    id: 'video',
    title: 'Video Generation',
    desc: 'Pick from mainstream video models; turn text or images into cinematic-grade video in one click.',
    icon: <Clapperboard className='size-5 text-emerald-300/90' strokeWidth={1.5} />,
    tint: 'border-emerald-400/20 bg-emerald-400/[0.07]',
  },
  {
    id: 'voice',
    title: 'Voice Synthesis',
    desc: 'Multilingual, multi-voice TTS from narration to dialogue — give your content a professional voice.',
    icon: <AudioLines className='size-5 text-amber-300/90' strokeWidth={1.5} />,
    tint: 'border-amber-400/20 bg-amber-400/[0.07]',
  },
]

/**
 * "Platform" section: the four core capability cards from the
 * reference landing page.
 */
export function PlatformCore() {
  const { t } = useTranslation()

  return (
    <section
      className='relative z-10 overflow-hidden bg-[#04070c] px-6 py-20 md:py-28'
      aria-label={t('Platform')}
    >
      <div
        aria-hidden
        className='pointer-events-none absolute inset-0'
        style={{
          background:
            'radial-gradient(ellipse 55% 45% at 50% 0%, oklch(0.32 0.1 190 / 22%) 0%, transparent 70%)',
        }}
      />
      <div className='relative mx-auto max-w-5xl'>
        <AnimateInView className='mb-14 text-center'>
          <span className='inline-flex items-center rounded-full border border-cyan-400/25 bg-cyan-400/[0.06] px-4 py-1.5 text-xs font-medium text-cyan-300'>
            {t('Platform')}
          </span>
          <h2 className='mt-6 text-3xl leading-snug font-bold tracking-tight text-white md:text-4xl'>
            {t('Four core capabilities,')}
            <br className='md:hidden' />
            <span className='bg-gradient-to-r from-cyan-300 to-teal-300 bg-clip-text text-transparent'>
              {' '}
              {t('one platform')}
            </span>
          </h2>
        </AnimateInView>

        <div className='grid gap-5 md:grid-cols-2'>
          {capabilities.map((cap, i) => (
            <AnimateInView
              key={cap.id}
              delay={i * 90}
              className={`group rounded-2xl border border-white/[0.07] bg-white/[0.03] p-6 backdrop-blur-sm transition-colors duration-300 hover:bg-white/[0.05] md:p-7 ${cap.tint.replace('border-', 'hover:border-')}`}
            >
              <div
                className={`mb-5 flex size-11 items-center justify-center rounded-xl border ${cap.tint}`}
              >
                {cap.icon}
              </div>
              <h3 className='text-base font-semibold text-white'>
                {t(cap.title)}
              </h3>
              <p className='mt-2 text-sm leading-relaxed text-slate-400'>
                {t(cap.desc)}
              </p>
            </AnimateInView>
          ))}
        </div>
      </div>
    </section>
  )
}