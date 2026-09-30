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
import { Check, ChevronDown } from 'lucide-react'

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'

export interface ChipOption {
  value: string
  label: string
}

interface ChipDropdownProps {
  value: string
  options: ChipOption[]
  onChange: (value: string) => void
  /** Optional group label shown in the popup header. */
  label?: string
  className?: string
}

/**
 * Compact chip styled like the reference creator bar: a dark rounded
 * pill with the current selection and a dropdown caret.
 */
export function ChipDropdown({
  value,
  options,
  onChange,
  label,
  className,
}: ChipDropdownProps) {
  const current = options.find((option) => option.value === value)

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <button
            type='button'
            className={cn(
              'group/chip inline-flex h-7 shrink-0 items-center gap-1 rounded-md border border-white/10 bg-white/[0.04] px-2 text-xs text-gray-200 transition-colors hover:border-cyan-400/40 hover:bg-white/[0.08]',
              className
            )}
          >
            <span className='max-w-28 truncate'>
              {current?.label ?? value}
            </span>
            <ChevronDown className='text-gray-500 size-3 transition-transform group-data-open/chip:rotate-180' />
          </button>
        }
      />
      <DropdownMenuContent align='start' sideOffset={6} className='w-40'>
        {label ? (
          <div className='text-gray-500 px-2 pb-1 pt-1.5 text-[11px]'>
            {label}
          </div>
        ) : null}
        {options.map((option) => (
          <DropdownMenuItem
            key={option.value}
            onClick={() => onChange(option.value)}
            className='rounded-md text-gray-200'
          >
            <span className='flex-1'>{option.label}</span>
            {option.value === value ? <Check className='size-3.5' /> : null}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
