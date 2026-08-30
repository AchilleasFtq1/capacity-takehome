import { StyleSheet, Text, View } from 'react-native';

import { Button, Empty, SectionTitle, TierChip } from '../components';
import type { Snapshot } from '../queries';
import { color, radius, space } from '../theme';
import type { Busy } from '../useAppData';

type Props = {
  snapshot: Snapshot;
  busy: Busy;
  onAccept: (requestId: string) => void;
  onDecline: (requestId: string, name: string) => void;
};

/**
 * R6 - the request inbox.
 *
 * Accept is offered on every request, including ones that are certain to fail
 * because the list is already full. That is deliberate: the refusal, with the
 * reason in it, is more use than a disabled button that explains nothing, and
 * the seat may well be free again by the time it is tapped.
 */
export function RequestsScreen({ snapshot, busy, onAccept, onDecline }: Props) {
  const { incomingRequests, outgoingRequests } = snapshot;
  const pendingOut = outgoingRequests.filter((r) => r.status === 'PENDING');
  const declinedOut = outgoingRequests.filter((r) => r.status === 'DECLINED');

  return (
    <View style={styles.screen}>
      <View>
        <SectionTitle>Waiting for you</SectionTitle>
        {incomingRequests.length === 0 ? (
          <Empty>No requests right now.</Empty>
        ) : (
          <View style={styles.rows}>
            {incomingRequests.map((r) => (
              <View key={r.id} style={styles.row}>
                <View style={styles.rowText}>
                  <Text style={styles.name}>{r.from.name}</Text>
                  <View style={styles.chipLine}>
                    <Text style={styles.sub}>wants to be your</Text>
                    <TierChip name={r.tier} small />
                  </View>
                </View>
                <View style={styles.actions}>
                  <Button
                    label="Accept"
                    kind="primary"
                    busy={busy[`accept:${r.id}`]}
                    onPress={() => onAccept(r.id)}
                  />
                  <Button
                    label="Decline"
                    busy={busy[`decline:${r.id}`]}
                    onPress={() => onDecline(r.id, r.from.name)}
                  />
                </View>
              </View>
            ))}
          </View>
        )}
      </View>

      <View>
        <SectionTitle>Sent by you</SectionTitle>
        {pendingOut.length === 0 && declinedOut.length === 0 ? (
          <Empty>Nothing outstanding. A pending request costs you no seat, so send as many as you like.</Empty>
        ) : (
          <View style={styles.rows}>
            {pendingOut.map((r) => (
              <View key={r.id} style={styles.row}>
                <View style={styles.rowText}>
                  <Text style={styles.name}>{r.to.name}</Text>
                  <View style={styles.chipLine}>
                    <Text style={styles.sub}>asked as</Text>
                    <TierChip name={r.tier} small />
                  </View>
                </View>
                <Text style={styles.status}>Waiting</Text>
              </View>
            ))}
            {declinedOut.map((r) => (
              <View key={r.id} style={[styles.row, styles.rowQuiet]}>
                <View style={styles.rowText}>
                  <Text style={[styles.name, styles.nameQuiet]}>{r.to.name}</Text>
                  <View style={styles.chipLine}>
                    <Text style={styles.sub}>asked as</Text>
                    <TierChip name={r.tier} small />
                  </View>
                </View>
                <Text style={[styles.status, styles.statusDeclined]}>Declined</Text>
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
  rows: { gap: space.sm },
  row: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: space.md,
    padding: space.md,
    borderRadius: radius.md,
    borderWidth: 1,
    borderColor: color.line,
    backgroundColor: color.surface,
  },
  rowQuiet: { backgroundColor: color.ground },
  rowText: { flex: 1, gap: 4 },
  chipLine: { flexDirection: 'row', alignItems: 'center', gap: 6, flexWrap: 'wrap' },
  name: { fontSize: 16, color: color.ink },
  nameQuiet: { color: color.muted },
  sub: { fontSize: 12, color: color.muted },
  actions: { gap: space.sm },
  status: { fontSize: 12, color: color.faint, textTransform: 'uppercase', letterSpacing: 0.8 },
  statusDeclined: { color: color.danger },
});
