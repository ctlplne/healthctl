import { createContext } from 'react'
import type { Region } from './records'
import { canonicalLocale } from './types'

export const RegionContext = createContext<Region | null>(null)

/** Only a named, scalar body measurement with an explicit unit is eligible. */
export function regionalMeasurement(
  row: { name: string; value: string; units: string; uncertain?: boolean },
  region: Region | null,
) {
  if (
    row.uncertain ||
    !region ||
    !['metric', 'us-customary'].includes(region.units)
  )
    return ''
  if (!/^[+-]?(?:\d+(?:\.\d+)?|\.\d+)$/.test(row.value.trim())) return ''
  const value = Number(row.value)
  if (!Number.isFinite(value)) return ''
  const name = row.name.trim().toLowerCase(),
    unit = row.units.trim().toLowerCase(),
    us = region.units === 'us-customary'
  let converted: number, target: string, quantity: string
  if (['patient weight', 'body weight'].includes(name)) {
    quantity = 'Body weight'
    if (us && ['kg', 'kilogram', 'kilograms'].includes(unit)) {
      converted = value / 0.45359237
      target = 'lb'
    } else if (!us && ['lb', 'lbs', 'pound', 'pounds'].includes(unit)) {
      converted = value * 0.45359237
      target = 'kg'
    } else return ''
  } else if (
    ['patient height', 'body height', 'standing height'].includes(name)
  ) {
    quantity = 'Height'
    if (us && unit === 'cm') {
      converted = value / 2.54
      target = 'in'
    } else if (!us && ['in', 'inch', 'inches'].includes(unit)) {
      converted = value * 2.54
      target = 'cm'
    } else if (us && unit === 'm') {
      converted = (value * 100) / 2.54
      target = 'in'
    } else return ''
  } else if (['body temperature', 'oral temperature'].includes(name)) {
    quantity = 'Body temperature'
    if (us && ['°c', 'celsius', 'deg c'].includes(unit)) {
      converted = (value * 9) / 5 + 32
      target = '°F'
    } else if (!us && ['°f', 'fahrenheit', 'deg f'].includes(unit)) {
      converted = ((value - 32) * 5) / 9
      target = '°C'
    } else return ''
  } else return ''
  if (!Number.isFinite(converted)) return ''
  const display = new Intl.NumberFormat(canonicalLocale(region.locale), {
    maximumFractionDigits: 1,
  }).format(converted)
  return `${quantity} ≈ ${display} ${target} (rounded to 0.1 ${target})`
}
