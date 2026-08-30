// Design tokens. Small on purpose - the brief puts visual polish out of scope,
// so this exists to keep the screens consistent, not to be a design system.

export const tier = {
  PINK: { label: 'Pink flag', ink: '#a3195b', wash: '#fbe7f0', edge: '#f0bed4' },
  BLUE: { label: 'Blue flag', ink: '#12518f', wash: '#e4eefb', edge: '#b9d3ef' },
  GREEN: { label: 'Green flag', ink: '#1d6b3a', wash: '#e3f2e8', edge: '#b6ddc4' },
} as const;

export type TierName = keyof typeof tier;

export const TIER_ORDER: readonly TierName[] = ['PINK', 'BLUE', 'GREEN'];

export const color = {
  ink: '#1a1a1a',
  muted: '#6b6b6b',
  faint: '#9a9a9a',
  line: '#e3e3e3',
  surface: '#ffffff',
  ground: '#f6f6f5',
  accent: '#16605c',
  accentWash: '#e1efed',
  danger: '#9a3b2e',
  dangerWash: '#f8e9e6',
  warn: '#8a6d1f',
  warnWash: '#fbf3dd',
} as const;

export const space = { xs: 4, sm: 8, md: 12, lg: 16, xl: 24 } as const;

export const radius = { sm: 6, md: 10, lg: 14, pill: 999 } as const;
