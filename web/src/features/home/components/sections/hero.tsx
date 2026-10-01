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
import { ArrowRight, Gift, Mail } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import { FloatingModels } from '../floating-models'
import { StarfieldCanvas } from '../starfield-canvas'

interface HeroProps {
  className?: string
  isAuthenticated?: boolean
}

function useTypewriter(words: string[]) {
  const [wordIndex, setWordIndex] = useState(0)
  const [text, setText] = useState('')
  const [deleting, setDeleting] = useState(false)

  useEffect(() => {
    const word = words[wordIndex % words.length] ?? ''
    let delay = deleting ? 140 : 300
    if (!deleting && text === word) delay = 1500
    else if (deleting && text === '') delay = 300

    const timer = setTimeout(() => {
      if (!deleting) {
        if (text === word) {
          setDeleting(true)
        } else {
          setText(word.slice(0, text.length + 1))
        }
      } else if (text === '') {
        setDeleting(false)
        setWordIndex((i) => (i + 1) % words.length)
      } else {
        setText(word.slice(0, text.length - 1))
      }
    }, delay)
    return () => clearTimeout(timer)
  }, [text, deleting, wordIndex, words])

  return text
}

/**
 * Full-screen nebula hero: self-drawn starfield canvas, scattered
 * floating model logos, typewriter headline and glowing CTAs.
 */
export function Hero(props: HeroProps) {
  const { t } = useTranslation()
  const words = [t('智'), t('创造'), t('想象'), t('进化'), t('释放灵感')]
  const typed = useTypewriter(words)

  return (
    <section className='relative z-10 flex min-h-[calc(100svh-4.5rem)] items-center overflow-hidden px-6 py-16 md:py-20'>
      {/* Deep-space base tone */}
      <div
        aria-hidden
        className='pointer-events-none absolute inset-0 -z-10'
        style={{
          background: [
            'radial-gradient(ellipse 70% 55% at 50% 42%, oklch(0.35 0.12 190 / 45%) 0%, transparent 65%)',
            'radial-gradient(ellipse 45% 40% at 18% 75%, oklch(0.30 0.10 160 / 30%) 0%, transparent 70%)',
            'radial-gradient(ellipse 50% 40% at 85% 20%, oklch(0.30 0.10 220 / 30%) 0%, transparent 70%)',
            'linear-gradient(180deg, #04070c 0%, #050a10 100%)',
          ].join(', '),
        }}
      />
      <StarfieldCanvas />
      <FloatingModels />

      <div className='relative z-10 mx-auto flex w-full max-w-3xl flex-col items-center text-center'>
        {/* Badge */}
        <div className='landing-animate-fade-up inline-flex items-center gap-2 rounded-full border border-emerald-400/30 bg-emerald-400/[0.07] px-4 py-1.5 text-xs font-medium text-emerald-300 opacity-0 backdrop-blur-sm' style={{ animationDelay: '0ms' }}>
          <span className='relative flex size-1.5'>
            <span className='absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75' />
            <span className='relative inline-flex size-1.5 rounded-full bg-emerald-400' />
          </span>
          {t('New Generation AI Platform')}
        </div>

        {/* Headline */}
        <h1 className='landing-animate-fade-up mt-7 text-[clamp(2.75rem,7vw,4.5rem)] leading-[1.12] font-black tracking-tight text-white opacity-0' style={{ animationDelay: '80ms' }}>
          {t('Let AI')}
          <span
            className='ml-3 inline-block min-w-[1.2em] bg-gradient-to-r from-emerald-300 via-teal-300 to-cyan-300 bg-clip-text text-transparent'
            aria-live='polite'
          >
            {typed}
            <span className='animate-pulse text-emerald-300/70'>|</span>
          </span>
        </h1>

        {/* Subtitle */}
        <p className='landing-animate-fade-up text-slate-400 mx-auto mt-6 max-w-2xl text-base leading-relaxed opacity-0 md:text-lg' style={{ animationDelay: '160ms' }}>
          {t(
            '500+ world-class models in one place: smart chat, image creation, video generation and AI agents — one platform, limitless possibility'
          )}
        </p>

        {/* Primary CTAs */}
        <div className='landing-animate-fade-up mt-9 flex flex-wrap items-center justify-center gap-4 opacity-0' style={{ animationDelay: '240ms' }}>
          <Button
            size='lg'
            className='group h-12 rounded-full bg-gradient-to-r from-emerald-400 to-cyan-400 px-8 text-sm font-bold text-slate-950 shadow-[0_0_30px_rgba(52,211,153,0.45)] transition-all hover:from-emerald-300 hover:to-cyan-300 hover:shadow-[0_0_40px_rgba(52,211,153,0.6)]'
            render={<Link to={props.isAuthenticated ? '/home' : '/sign-up'} />}
          >
            {t('Get Started Now')}
            <ArrowRight className='ml-2 size-4 transition-transform duration-200 group-hover:translate-x-0.5' />
          </Button>
          <Button
            size='lg'
            variant='outline'
            className='h-12 rounded-full border-white/15 bg-white/[0.04] px-7 text-sm font-medium text-slate-200 backdrop-blur-sm hover:border-white/30 hover:bg-white/[0.08]'
            render={<a href='mailto:business@kejiapi.com' />}
          >
            <Mail className='mr-2 size-4 text-slate-400' />
            {t('Business Cooperation')}
          </Button>
        </div>

        {/* Free gift banner */}
        <Link
          to={props.isAuthenticated ? '/home' : '/sign-up'}
          className='landing-animate-fade-up mt-6 inline-flex items-center gap-2 rounded-full border border-cyan-400/25 bg-cyan-400/[0.06] px-5 py-2 text-xs font-medium text-cyan-200 opacity-0 backdrop-blur-sm transition-colors hover:border-cyan-400/50 hover:bg-cyan-400/10'
          style={{ animationDelay: '320ms' }}
        >
          <Gift className='size-3.5 text-cyan-300' />
          {t('Sign up to claim the free starter pack: AI painting / video / chat all free to use')}
          <ArrowRight className='size-3.5' />
        </Link>

        {/* Hero stats */}
        <div className='landing-animate-fade-up mt-12 flex items-center justify-center gap-10 opacity-0 md:gap-16' style={{ animationDelay: '400ms' }}>
          <div className='flex flex-col items-center'>
            <span className='text-2xl font-bold text-white md:text-3xl'>500+</span>
            <span className='mt-1 text-xs text-slate-500'>{t('AI Models')}</span>
          </div>
          <div className='h-8 w-px bg-white/10' />
          <div className='flex flex-col items-center'>
            <span className='text-2xl font-bold text-white md:text-3xl'>10K+</span>
            <span className='mt-1 text-xs text-slate-500'>{t('Creators')}</span>
          </div>
          <div className='h-8 w-px bg-white/10' />
          <div className='flex flex-col items-center'>
            <span className='text-2xl font-bold text-emerald-300 md:text-3xl'>∞</span>
            <span className='mt-1 text-xs text-slate-500'>{t('Creativity')}</span>
          </div>
        </div>
      </div>
    </section>
  )
}