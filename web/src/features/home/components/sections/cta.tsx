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
import { ArrowRight, Headphones } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { AnimateInView } from '@/components/animate-in-view'
import { Button } from '@/components/ui/button'

interface CTAProps {
  className?: string
  isAuthenticated?: boolean
}

export function CTA(props: CTAProps) {
  const { t } = useTranslation()

  return (
    <section className='relative z-10 overflow-hidden bg-[#04070c] px-6 py-24 md:py-32'>
      <div
        aria-hidden
        className='pointer-events-none absolute inset-0'
        style={{
          background: [
            'radial-gradient(ellipse 60% 50% at 50% 100%, oklch(0.35 0.12 190 / 30%) 0%, transparent 70%)',
            'radial-gradient(ellipse 40% 40% at 20% 20%, oklch(0.3 0.1 220 / 18%) 0%, transparent 70%)',
          ].join(', '),
        }}
      />
      <div
        aria-hidden
        className='pointer-events-none absolute inset-0 opacity-30'
        style={{
          backgroundImage:
            'linear-gradient(rgba(148, 163, 184, 0.05) 1px, transparent 1px), linear-gradient(90deg, rgba(148, 163, 184, 0.05) 1px, transparent 1px)',
          backgroundSize: '44px 44px',
        }}
      />

      <AnimateInView
        className='relative mx-auto max-w-2xl text-center'
        animation='scale-in'
      >
        <h2 className='text-3xl leading-tight font-black tracking-tight text-white md:text-5xl'>
          {t('Start now')}
          <br />
          <span className='bg-gradient-to-r from-cyan-300 via-teal-300 to-emerald-300 bg-clip-text text-transparent'>
            {t('your AI creation journey')}
          </span>
        </h2>
        <p className='mx-auto mt-6 max-w-md text-sm text-slate-400 md:text-base'>
          {t(
            '30+ top models / AI agents / inspiration plaza — all in one stop'
          )}
        </p>
        <div className='mt-9 flex flex-wrap items-center justify-center gap-4'>
          <Button
            size='lg'
            className='group h-12 rounded-full bg-gradient-to-r from-cyan-400 to-teal-400 px-8 text-sm font-bold text-slate-950 shadow-[0_0_30px_rgba(34,211,238,0.35)] transition-all hover:from-cyan-300 hover:to-teal-300'
            render={<Link to={props.isAuthenticated ? '/home' : '/sign-up'} />}
          >
            {props.isAuthenticated
              ? t('Get Started Now')
              : t('Sign Up Now')}
            <ArrowRight className='ml-2 size-4 transition-transform duration-200 group-hover:translate-x-0.5' />
          </Button>
          <Button
            size='lg'
            variant='outline'
            className='h-12 rounded-full border-white/15 bg-white/[0.04] px-7 text-sm font-medium text-slate-200 backdrop-blur-sm hover:border-white/30 hover:bg-white/[0.08]'
            render={<a href='mailto:support@kejiapi.com' />}
          >
            <Headphones className='mr-2 size-4 text-slate-400' />
            {t('Contact Support')}
          </Button>
        </div>
      </AnimateInView>
    </section>
  )
}