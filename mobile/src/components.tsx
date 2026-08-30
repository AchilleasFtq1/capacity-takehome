import type { ReactNode } from 'react';
import { ActivityIndicator, Pressable, StyleSheet, Text, View } from 'react-native';

import type { Notice } from './useAppData';
import { color, radius, space, tier as tierTokens, type TierName } from './theme';

export function TierChip({ name, small }: { name: TierName; small?: boolean }) {
  const t = tierTokens[name];
  return (
    <View style={[styles.chip, { backgroundColor: t.wash, borderColor: t.edge }, small && styles.chipSmall]}>
      <Text style={[styles.chipText, { color: t.ink }, small && styles.chipTextSmall]}>{t.label}</Text>
    </View>
  );
}

/**
 * A used/cap readout that stays honest when used exceeds cap - a lowered cap or
 * a merge can put someone over, and the brief is explicit that nothing may
 * assume used <= cap. The bar clamps; the number does not.
 */
export function Meter({
  used,
  cap,
  label,
  tint = color.accent,
}: {
  used: number;
  cap: number;
  label: string;
  tint?: string;
}) {
  const over = used > cap;
  const fraction = cap > 0 ? Math.min(used / cap, 1) : used > 0 ? 1 : 0;
  const barColor = over ? color.danger : tint;

  return (
    <View style={styles.meter}>
      <View style={styles.meterHead}>
        <Text style={styles.meterLabel}>{label}</Text>
        <Text style={[styles.meterCount, over && { color: color.danger }]}>
          {used} / {cap}
          {over ? '  over' : ''}
        </Text>
      </View>
      <View style={styles.track}>
        <View style={[styles.fill, { width: `${fraction * 100}%`, backgroundColor: barColor }]} />
      </View>
    </View>
  );
}

/**
 * The refusal surface. The brief asks that a failed action says why in plain
 * words, so this is deliberately loud, sticky until dismissed, and shows the
 * server's sentence untouched.
 */
export function NoticeBanner({ notice, onDismiss }: { notice: Notice; onDismiss: () => void }) {
  const tone =
    notice.kind === 'ok'
      ? { bg: color.accentWash, fg: color.accent, title: 'Done' }
      : notice.kind === 'refusal'
        ? { bg: color.warnWash, fg: color.warn, title: "Can't do that" }
        : { bg: color.dangerWash, fg: color.danger, title: 'Something broke' };

  return (
    <View style={[styles.banner, { backgroundColor: tone.bg }]} accessibilityLiveRegion="polite">
      <View style={styles.bannerBody}>
        <Text style={[styles.bannerTitle, { color: tone.fg }]}>{tone.title}</Text>
        <Text style={[styles.bannerText, { color: tone.fg }]}>{notice.text}</Text>
      </View>
      <Pressable onPress={onDismiss} hitSlop={10} accessibilityRole="button" accessibilityLabel="Dismiss">
        <Text style={[styles.bannerClose, { color: tone.fg }]}>×</Text>
      </Pressable>
    </View>
  );
}

export function Button({
  label,
  onPress,
  kind = 'plain',
  busy,
  disabled,
}: {
  label: string;
  onPress: () => void;
  kind?: 'primary' | 'plain' | 'danger';
  busy?: boolean;
  disabled?: boolean;
}) {
  const off = disabled || busy;
  return (
    <Pressable
      onPress={onPress}
      disabled={off}
      accessibilityRole="button"
      accessibilityLabel={label}
      accessibilityState={{ disabled: !!off, busy: !!busy }}
      style={({ pressed }) => [
        styles.button,
        kind === 'primary' && styles.buttonPrimary,
        kind === 'danger' && styles.buttonDanger,
        off && styles.buttonOff,
        pressed && !off && styles.buttonPressed,
      ]}
    >
      {busy ? (
        <ActivityIndicator size="small" color={kind === 'plain' ? color.muted : '#fff'} />
      ) : (
        <Text
          style={[
            styles.buttonText,
            kind === 'primary' && styles.buttonTextOn,
            kind === 'danger' && styles.buttonTextOn,
          ]}
        >
          {label}
        </Text>
      )}
    </Pressable>
  );
}

export function Segmented<T extends string>({
  options,
  value,
  onChange,
}: {
  options: { value: T; label: string; badge?: number }[];
  value: T;
  onChange: (v: T) => void;
}) {
  return (
    <View style={styles.segmented} accessibilityRole="tablist">
      {options.map((o) => {
        const active = o.value === value;
        return (
          <Pressable
            key={o.value}
            onPress={() => onChange(o.value)}
            accessibilityRole="tab"
            accessibilityState={{ selected: active }}
            style={[styles.segment, active && styles.segmentActive]}
          >
            <Text style={[styles.segmentText, active && styles.segmentTextActive]}>{o.label}</Text>
            {o.badge ? (
              <View style={styles.badge}>
                <Text style={styles.badgeText}>{o.badge}</Text>
              </View>
            ) : null}
          </Pressable>
        );
      })}
    </View>
  );
}

export function Empty({ children }: { children: ReactNode }) {
  return <Text style={styles.empty}>{children}</Text>;
}

export function SectionTitle({ children, right }: { children: ReactNode; right?: ReactNode }) {
  return (
    <View style={styles.sectionTitle}>
      <Text style={styles.sectionTitleText}>{children}</Text>
      {right}
    </View>
  );
}

export const styles = StyleSheet.create({
  chip: {
    alignSelf: 'flex-start',
    paddingHorizontal: space.sm,
    paddingVertical: 3,
    borderRadius: radius.pill,
    borderWidth: 1,
  },
  chipSmall: { paddingHorizontal: 6, paddingVertical: 1 },
  chipText: { fontSize: 12, fontWeight: '600' },
  chipTextSmall: { fontSize: 11 },

  meter: { gap: 5 },
  meterHead: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'baseline' },
  meterLabel: { fontSize: 13, color: color.ink, fontWeight: '600' },
  meterCount: { fontSize: 13, color: color.muted, fontVariant: ['tabular-nums'] },
  track: { height: 6, borderRadius: radius.pill, backgroundColor: color.line, overflow: 'hidden' },
  fill: { height: 6, borderRadius: radius.pill },

  banner: {
    flexDirection: 'row',
    alignItems: 'flex-start',
    gap: space.md,
    padding: space.md,
    borderRadius: radius.md,
  },
  bannerBody: { flex: 1, gap: 2 },
  bannerTitle: { fontSize: 11, fontWeight: '700', textTransform: 'uppercase', letterSpacing: 0.8 },
  bannerText: { fontSize: 14, lineHeight: 20 },
  bannerClose: { fontSize: 22, lineHeight: 22, fontWeight: '400' },

  button: {
    minHeight: 36,
    minWidth: 72,
    paddingHorizontal: space.md,
    paddingVertical: space.sm,
    borderRadius: radius.sm,
    borderWidth: 1,
    borderColor: color.line,
    backgroundColor: color.surface,
    alignItems: 'center',
    justifyContent: 'center',
  },
  buttonPrimary: { backgroundColor: color.accent, borderColor: color.accent },
  buttonDanger: { backgroundColor: color.danger, borderColor: color.danger },
  buttonOff: { opacity: 0.45 },
  buttonPressed: { opacity: 0.7 },
  buttonText: { fontSize: 14, color: color.ink, fontWeight: '500' },
  buttonTextOn: { color: '#fff' },

  segmented: {
    flexDirection: 'row',
    backgroundColor: color.ground,
    borderRadius: radius.md,
    padding: 3,
    gap: 3,
  },
  segment: {
    flex: 1,
    flexDirection: 'row',
    gap: 6,
    alignItems: 'center',
    justifyContent: 'center',
    paddingVertical: space.sm,
    borderRadius: radius.sm,
  },
  segmentActive: { backgroundColor: color.surface },
  segmentText: { fontSize: 14, color: color.muted },
  segmentTextActive: { color: color.ink, fontWeight: '600' },
  badge: {
    minWidth: 18,
    paddingHorizontal: 5,
    paddingVertical: 1,
    borderRadius: radius.pill,
    backgroundColor: color.accent,
  },
  badgeText: { color: '#fff', fontSize: 11, fontWeight: '700', textAlign: 'center' },

  empty: { fontSize: 14, color: color.faint, lineHeight: 20, paddingVertical: space.sm },

  sectionTitle: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: space.sm,
  },
  sectionTitleText: {
    fontSize: 12,
    textTransform: 'uppercase',
    letterSpacing: 1,
    color: color.faint,
    fontWeight: '600',
  },
});
