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
import { Link } from '@tanstack/react-router'
import {
  ArrowRight,
  AudioLines,
  Clapperboard,
  Palette,
  Sparkles,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { AnimateInView } from '@/components/animate-in-view'

const cards = [
  {
    id: 'comic',
    num: '01',
    title: 'One-click comic drama generation',
    desc: 'Input a script or reference images; AI splits shots, generates characters, voices and composes the video — the whole flow in one click.',
    icon: <Clapperboard className='size-5 text-emerald-300/80' strokeWidth={1.5} />,
  },
  {
    id: 'long-video',
    num: '02',
    title: 'Smart long-video creation',
    desc: 'Multiple reference images + text description precisely control content and duration, producing coherent, professional-grade long videos.',
    icon: <Sparkles className='size-5 text-cyan-300/80' strokeWidth={1.5} />,
  },
  {
    id: 'style',
    num: '03',
    title: 'One-click style switching',
    desc: 'Cyberpunk, ink-wash, 3D animation, photoreal cinematic… switch between tons of styles with no limits.',
    icon: <Palette className='size-5 text-violet-300/80' strokeWidth={1.5} />,
  },
  {
    id: 'voice',
    num: '04',
    title: 'Auto voice matching for every frame',
    desc: 'Smart TTS + rhythm matching; pick narration or drama style and give your content a soul.',
    icon: <AudioLines className='size-5 text-amber-300/80' strokeWidth={1.5} />,
  },
]

/**
 * "AI agents" section: numbered capability cards shown on the
 * reference landing page below the hero.
 */
export function AgentShowcase() {
  const { t } = useTranslation()

  return (
    <section
      className='relative z-10 overflow-hidden bg-[#050a10] px-6 py-20 md:py-28'
      aria-label={t('AI Agents')}
    >
      <div
        aria-hidden
        className='pointer-events-none absolute inset-0 opacity-40'
        style={{
          backgroundImage:
            'linear-gradient(rgba(148, 163, 184, 0.05) 1px, transparent 1px), linear-gradient(90deg, rgba(148, 163, 184, 0.05) 1px, transparent 1px)',
          backgroundSize: '44px 44px',
          maskImage:
            'radial-gradient(ellipse 80% 70% at 50% 30%, black 30%, transparent 100%)',
        }}
      />
      <div className='relative mx-auto max-w-6xl'>
        <AnimateInView className='mb-14 text-center'>
          <span className='inline-flex items-center gap-2 rounded-full border border-emerald-400/25 bg-emerald-400/[0.06] px-4 py-1.5 text-xs font-medium text-emerald-300'>
            <Sparkles className='size-3.5' />
            {t('AI Agents')}
          </span>
          <h2 className='mt-6 text-3xl leading-snug font-bold tracking-tight text-white md:text-4xl'>
            {t('Not just a tool')}
            <br />
            <span className='bg-gradient-to-r from-emerald-300 via-teal-300 to-cyan-300 bg-clip-text text-transparent'>
              {t('your creative partner')}
            </span>
          </h2>
          <p className='mx-auto mt-5 max-w-xl text-sm leading-relaxed text-slate-400 md:text-base'>
            {t(
              'From copy to video, from inspiration to finished product — AI agents handle the whole workflow, boosting efficiency 10x.'
            )}
          </p>
        </AnimateInView>

        <div className='grid gap-5 md:grid-cols-2'>
          {cards.map((card, i) => (
            <AnimateInView
              key={card.id}
              delay={i * 90}
              className='group rounded-2xl border border-white/[0.07] bg-white/[0.03] p-6 backdrop-blur-sm transition-colors duration-300 hover:border-emerald-400/25 hover:bg-white/[0.05] md:p-7'
            >
              <div className='mb-4 flex items-start justify-between'>
                <span className='flex size-9 items-center justify-center rounded-lg border border-white/10 bg-white/[0.04] text-xs font-bold text-slate-400 tabular-nums'>
                  {card.num}
                </span>
                <div className='flex size-10 items-center justify-center rounded-xl border border-white/[0.07] bg-white/[0.03]'>
                  {card.icon}
                </div>
              </div>
              <h3 className='text-base font-semibold text-white'>
                {t(card.title)}
              </h3>
              <p className='mt-2 text-sm leading-relaxed text-slate-400'>
                {t(card.desc)}
              </p>
              <Link
                to='/home'
                className='mt-4 inline-flex items-center gap-1 text-xs font-medium text-cyan-300 transition-colors group-hover:text-cyan-200'
              >
                {t('Try it now')}
                <ArrowRight className='size-3.5 transition-transform duration-200 group-hover:translate-x-0.5' />
              </Link>
            </AnimateInView>
          ))}
        </div>
      </div>
    </section>
  )
}