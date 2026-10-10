const HUE_START = 170
const HUE_END = 330
const SATURATION = 0.55
const DARK_LIGHTNESS = 0.5
const LIGHT_LIGHTNESS = 0.8
const HEX_BASE = 16
const SECTOR_DEGREES = 60
const SINGLE_HUE = HUE_START

const channel = (hue: number, lightness: number, offset: number) => {
  const k = (offset + hue / (SECTOR_DEGREES / 2)) % 12
  const a = SATURATION * Math.min(lightness, 1 - lightness)
  return Math.round(255 * (lightness - a * Math.max(-1, Math.min(k - 3, 9 - k, 1))))
}
const hex = (value: number) => value.toString(HEX_BASE).padStart(2, '0')

export const segmentColors = (count: number) =>
  Array.from({ length: count }, (_, index) => {
    const hue = count > 1 ? HUE_START + ((HUE_END - HUE_START) * index) / (count - 1) : SINGLE_HUE
    const lightness = index % 2 ? LIGHT_LIGHTNESS : DARK_LIGHTNESS
    return `#${[0, 8, 4].map(offset => hex(channel(hue, lightness, offset))).join('')}`
  })
