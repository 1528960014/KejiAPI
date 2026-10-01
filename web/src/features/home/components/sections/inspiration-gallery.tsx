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
import { ChevronDown, Eye, Heart } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { AnimateInView } from '@/components/animate-in-view'
import { Button } from '@/components/ui/button'

const CDN = 'https://cos.lingkeai.vip/uploads'
const TOS = 'https://tos.lingkeai.vip/uploads'

const works = [
  {
    id: 'w1',
    src: `${CDN}/2026.07/29/20260729200007_18c6c0e8ad40ada0c3c6.png`,
    title: '物美价廉',
    likes: 3,
    views: 0,
    tags: [],
  },
  {
    id: 'w2',
    src: `${CDN}/2026.06/02/20260602230627_18b54bff45b1b094b1fe.png`,
    title: '美女',
    likes: 902,
    views: 2,
    tags: ['人像摄影', '仿真人', '时尚穿搭'],
  },
  {
    id: 'w3',
    src: `${CDN}/2026.01/22/20260122192913_188d0a1e50f6f400ca53.jpg`,
    title: '暮色街头：蓝调时刻的随性漫步',
    likes: 931,
    views: 11,
    tags: ['人像摄影', '写实摄影', '潮流文化'],
  },
  {
    id: 'w4',
    src: `${TOS}/2026.09/27/20260927090143_18d907ccead5d473be6e.png`,
    title: '印花提取',
    likes: 900,
    views: 0,
    tags: ['传统纹样', '神话传说', '艺术插画'],
  },
  {
    id: 'w5',
    src: `${CDN}/2026.07/25/20260725070318_18c55c324748b94d5f6b.png`,
    title: '演唱会等待入场的粉丝们',
    likes: 765,
    views: 0,
    tags: ['主观视角', '人物群像', '氛围感'],
  },
  {
    id: 'w6',
    src: `${CDN}/2026.01/22/20260122192716_188d0a02e6e37428f5bf.jpg`,
    title: '光影流转：卢米埃尔电影节复古海报',
    likes: 943,
    views: 13,
    tags: ['创意海报', '唯美光影', '复古美学'],
  },
  {
    id: 'w7',
    src: `${CDN}/2026.05/29/20260529173706_18b3ffb40bd74040e147.png`,
    title: '瑰丽桃粉：次世代仙气国风少女3D人设卡',
    likes: 1116,
    views: 32,
    tags: ['CG渲染', '三视图', '国风美学'],
  },
  {
    id: 'w8',
    src: `${CDN}/2026.05/19/20260519210127_18b0f90d07b29740e7cf.png`,
    title: '卧室里的女孩',
    likes: 999,
    views: 22,
    tags: ['人像摄影', '仰拍视角', '写实摄影'],
  },
  {
    id: 'w9',
    src: `${CDN}/2026.06/11/20260611100222_18b7e46ee03ff393ae0c.png`,
    title: '苗族服装租赁海报喷布门头',
    likes: 11,
    views: 1,
    tags: ['产品海报', '商业海报'],
  },
  {
    id: 'w10',
    src: `${CDN}/2026.06/24/20260624184235_18bbfe5d9e293d01fd53.png`,
    title: '生成一张男性街头穿搭博主的自拍',
    likes: 979,
    views: 0,
    tags: ['三视图', '立绘', '美式复古'],
  },
  {
    id: 'w11',
    src: `${CDN}/2026.07/03/20260703232255_18bed0e2f3be5fd4e3ab.png`,
    title: '鸟瞰效果',
    likes: 933,
    views: 5,
    tags: ['俯瞰视角', '园艺设计', '建筑设计'],
  },
  {
    id: 'w12',
    src: `${CDN}/2026.06/03/20260603133231_18b57b420ace98b0a02d.png`,
    title: '国风3D动漫风格 CG 质感',
    likes: 936,
    views: 0,
    tags: ['3D国漫', 'CG质感', '国风'],
  },
]

/**
 * "Inspiration plaza" section: masonry gallery of community AI
 * creations with like/view stats and tags.
 */
export function InspirationGallery() {
  const { t } = useTranslation()

  return (
    <section
      className='relative z-10 overflow-hidden bg-[#050a10] px-6 py-20 md:py-28'
      aria-label={t('Inspiration Plaza')}
    >
      <div
        aria-hidden
        className='pointer-events-none absolute inset-0 opacity-40'
        style={{
          backgroundImage:
            'linear-gradient(rgba(148, 163, 184, 0.05) 1px, transparent 1px), linear-gradient(90deg, rgba(148, 163, 184, 0.05) 1px, transparent 1px)',
          backgroundSize: '44px 44px',
          maskImage:
            'radial-gradient(ellipse 80% 60% at 50% 20%, black 30%, transparent 100%)',
        }}
      />
      <div className='relative mx-auto max-w-6xl'>
        <AnimateInView className='mb-12 text-center'>
          <span className='inline-flex items-center rounded-full border border-emerald-400/25 bg-emerald-400/[0.06] px-4 py-1.5 text-xs font-medium text-emerald-300'>
            {t('Inspiration')}
          </span>
          <h2 className='mt-6 text-3xl leading-snug font-bold tracking-tight text-white md:text-4xl'>
            {t('Inspiration Plaza')}
            <br />
            <span className='bg-gradient-to-r from-emerald-300 via-teal-300 to-cyan-300 bg-clip-text text-transparent'>
              {t('See what everyone is creating with AI')}
            </span>
          </h2>
        </AnimateInView>

        <div className='columns-1 gap-4 sm:columns-2 lg:columns-3'>
          {works.map((w, i) => (
            <AnimateInView
              key={w.id}
              delay={(i % 6) * 70}
              className='mb-4 break-inside-avoid overflow-hidden rounded-xl border border-white/[0.07] bg-white/[0.03] transition-colors duration-300 hover:border-cyan-400/25'
            >
              <div className='relative'>
                <img
                  src={w.src}
                  alt={w.title}
                  loading='lazy'
                  className='w-full object-cover'
                />
                <div
                  aria-hidden
                  className='pointer-events-none absolute inset-0 bg-gradient-to-t from-black/20 to-transparent opacity-0 transition-opacity duration-300 group-hover:opacity-100'
                />
              </div>
              <div className='p-3.5'>
                <div className='mb-2 flex items-center gap-3 text-[11px] text-slate-500'>
                  <span className='inline-flex items-center gap-1'>
                    <Heart className='size-3 text-rose-400/70' />
                    {w.likes}
                  </span>
                  <span className='inline-flex items-center gap-1'>
                    <Eye className='size-3' />
                    {w.views}
                  </span>
                </div>
                <p className='line-clamp-2 text-sm leading-snug font-medium text-slate-200'>
                  {t(w.title)}
                </p>
                {w.tags.length > 0 && (
                  <div className='mt-2.5 flex flex-wrap gap-1.5'>
                    {w.tags.map((tag) => (
                      <span
                        key={tag}
                        className='rounded-md border border-cyan-400/20 bg-cyan-400/[0.05] px-2 py-0.5 text-[11px] text-cyan-200/90'
                      >
                        {t(tag)}
                      </span>
                    ))}
                  </div>
                )}
              </div>
            </AnimateInView>
          ))}
        </div>

        <div className='mt-10 flex justify-center'>
          <Button
            variant='outline'
            className='hover:border-cyan-400/40 hover:bg-cyan-400/10 border-white/10 bg-white/[0.03] text-slate-300 rounded-full px-6'
            render={<Link to='/home' />}
          >
            {t('Load more inspiration')}
            <ChevronDown className='ml-1.5 size-4' />
          </Button>
        </div>
      </div>
    </section>
  )
}