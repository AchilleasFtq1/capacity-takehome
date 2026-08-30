import { useMemo, useState } from 'react';
import { Pressable, StyleSheet, Text, View } from 'react-native';

import { Button, Empty, SectionTitle } from '../components';
import type { Snapshot } from '../queries';
import { color, radius, space, TIER_ORDER, tier as tierTokens, type TierName } from '../theme';
import type { Busy } from '../useAppData';

type Props = {
  snapshot: Snapshot;
  busy: Busy;
  onSend: (toUserId: string, tier: TierName, name: string) => void;
};

/**
 * R1 - send a request to a named tier.
 *
 * Anyone already a contact, or already mid-request either way, is filtered out
 * here so the obvious duplicates never get sent. The server still refuses them
 * independently; this list is a convenience, not the enforcement.
 */
export function AddScreen({ snapshot, busy, onSend }: Props) {
  const [tier, setTier] = useState<TierName>('GREEN');

  const available = useMemo(() => {
    const taken = new Set<string>([snapshot.me.id]);
    snapshot.contacts.forEach((c) => taken.add(c.user.id));
    snapshot.incomingRequests.forEach((r) => taken.add(r.from.id));
    snapshot.outgoingRequests.filter((r) => r.status === 'PENDING').forEach((r) => taken.add(r.to.id));
    return snapshot.users.filter((u) => !taken.has(u.id));
  }, [snapshot]);

  return (
    <View style={styles.screen}>
      <View>
        <SectionTitle>Ask them to be your</SectionTitle>
        <View style={styles.tiers}>
          {TIER_ORDER.map((t) => {
            const active = t === tier;
            const tokens = tierTokens[t];
            const cap = snapshot.capacity.tiers.find((x) => x.tier === t);
            return (
              <Pressable
                key={t}
                onPress={() => setTier(t)}
                accessibilityRole="radio"
                accessibilityState={{ selected: active }}
                style={[
                  styles.tierCard,
                  { borderColor: active ? tokens.ink : color.line },
                  active && { backgroundColor: tokens.wash },
                ]}
              >
                <Text style={[styles.tierName, active && { color: tokens.ink }]}>{tokens.label}</Text>
                <Text style={styles.tierCount}>
                  {cap ? `${cap.used} / ${cap.cap}` : '—'}
                </Text>
              </Pressable>
            );
          })}
        </View>
        <Text style={styles.note}>
          Sending costs nothing. The seat is only counted when they accept, and it is checked
          against both of you then.
        </Text>
      </View>

      <View>
        <SectionTitle>Send to</SectionTitle>
        {available.length === 0 ? (
          <Empty>Everyone here is already a contact or has a request in flight.</Empty>
        ) : (
          <View style={styles.rows}>
            {available.map((u) => (
              <View key={u.id} style={styles.row}>
                <Text style={styles.name}>{u.name}</Text>
                <Button
                  label="Send"
                  kind="primary"
                  busy={busy[`send:${u.id}`]}
                  onPress={() => onSend(u.id, tier, u.name)}
                />
              </View>
            ))}
          </View>
        )}
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  screen: { gap: space.xl },
  tiers: { flexDirection: 'row', gap: space.sm },
  tierCard: {
    flex: 1,
    gap: 2,
    padding: space.md,
    borderRadius: radius.md,
    borderWidth: 1.5,
    backgroundColor: color.surface,
  },
  tierName: { fontSize: 13, fontWeight: '600', color: color.ink },
  tierCount: { fontSize: 12, color: color.muted, fontVariant: ['tabular-nums'] },
  note: { fontSize: 12, lineHeight: 17, color: color.muted, marginTop: space.md },

  rows: { gap: space.sm },
  row: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: space.md,
    borderRadius: radius.md,
    borderWidth: 1,
    borderColor: color.line,
    backgroundColor: color.surface,
  },
  name: { fontSize: 16, color: color.ink },
});
