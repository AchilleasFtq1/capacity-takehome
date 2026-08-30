import { useState } from 'react';
import { Modal, Pressable, StyleSheet, Text, View } from 'react-native';

import { Button, Empty, Meter, SectionTitle, TierChip } from '../components';
import type { Contact, Snapshot } from '../queries';
import { color, radius, space, TIER_ORDER, tier as tierTokens, type TierName } from '../theme';
import type { Busy } from '../useAppData';

type Props = {
  snapshot: Snapshot;
  busy: Busy;
  onMove: (contactId: string, tier: TierName, name: string) => void;
  onRemove: (contactId: string, name: string) => void;
};

/**
 * R5 - contacts grouped by tier, with live used/cap per tier and the shared
 * budget above them.
 *
 * The budget sits at the top and the tiers below it on purpose: that is the
 * order the rules are applied in, and a screen that showed three tier meters
 * without the budget would make "Pink is empty but you still cannot add anyone"
 * look like a bug.
 */
export function PeopleScreen({ snapshot, busy, onMove, onRemove }: Props) {
  const [editing, setEditing] = useState<Contact | null>(null);
  const { capacity, contacts } = snapshot;
  const spare = capacity.budgetCap - capacity.budgetUsed;

  return (
    <View style={styles.screen}>
      <View style={styles.budget}>
        <Meter used={capacity.budgetUsed} cap={capacity.budgetCap} label="Shared budget" />
        <Text style={styles.budgetNote}>
          {spare > 0
            ? `${spare} seat${spare === 1 ? '' : 's'} free. The tiers below add up to more than the budget, so the budget is what actually stops you.`
            : 'No seats free. Every tier is closed until you remove someone, even the empty ones.'}
        </Text>
      </View>

      {TIER_ORDER.map((name) => {
        const t = capacity.tiers.find((x) => x.tier === name);
        const rows = contacts.filter((c) => c.tier === name);
        return (
          <View key={name} style={styles.tierBlock}>
            <Meter
              used={t?.used ?? rows.length}
              cap={t?.cap ?? 0}
              label={tierTokens[name].label}
              tint={tierTokens[name].ink}
            />
            {rows.length === 0 ? (
              <Empty>Nobody here yet.</Empty>
            ) : (
              <View style={styles.rows}>
                {rows.map((c) => (
                  <Pressable
                    key={c.id}
                    onPress={() => setEditing(c)}
                    accessibilityRole="button"
                    accessibilityLabel={`${c.user.name}, ${tierTokens[name].label}. Change tier or remove.`}
                    style={({ pressed }) => [styles.row, pressed && styles.rowPressed]}
                  >
                    <Text style={styles.rowName}>{c.user.name}</Text>
                    <Text style={styles.rowHint}>Edit</Text>
                  </Pressable>
                ))}
              </View>
            )}
          </View>
        );
      })}

      <ContactSheet
        contact={editing}
        busy={busy}
        onClose={() => setEditing(null)}
        onMove={(id, t, name) => {
          setEditing(null);
          onMove(id, t, name);
        }}
        onRemove={(id, name) => {
          setEditing(null);
          onRemove(id, name);
        }}
      />
    </View>
  );
}

/**
 * R3 and R4 in one sheet. Moving is offered for every other tier without
 * pre-checking whether it will fit: the server owns that decision, and a client
 * that greys out a destination is a second copy of the rule that will drift.
 */
function ContactSheet({
  contact,
  busy,
  onClose,
  onMove,
  onRemove,
}: {
  contact: Contact | null;
  busy: Busy;
  onClose: () => void;
  onMove: (contactId: string, tier: TierName, name: string) => void;
  onRemove: (contactId: string, name: string) => void;
}) {
  return (
    <Modal
      visible={contact !== null}
      transparent
      animationType="slide"
      onRequestClose={onClose}
      accessibilityViewIsModal
    >
      <Pressable style={styles.backdrop} onPress={onClose} accessibilityLabel="Close" />
      <View style={styles.sheet}>
        {contact && (
          <>
            <Text style={styles.sheetName}>{contact.user.name}</Text>
            <View style={styles.sheetChip}>
              <TierChip name={contact.tier} />
            </View>

            <SectionTitle>Move to</SectionTitle>
            <View style={styles.sheetTiers}>
              {TIER_ORDER.filter((t) => t !== contact.tier).map((t) => (
                <Button
                  key={t}
                  label={tierTokens[t].label}
                  busy={busy[`move:${contact.id}`]}
                  onPress={() => onMove(contact.id, t, contact.user.name)}
                />
              ))}
            </View>

            <View style={styles.sheetFooter}>
              <Button
                label="Remove"
                kind="danger"
                busy={busy[`remove:${contact.id}`]}
                onPress={() => onRemove(contact.id, contact.user.name)}
              />
              <Button label="Cancel" onPress={onClose} />
            </View>
            <Text style={styles.sheetNote}>
              Removing frees the seat for both of you. Moving changes only your side.
            </Text>
          </>
        )}
      </View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  screen: { gap: space.xl },

  budget: {
    gap: space.sm,
    padding: space.md,
    borderRadius: radius.md,
    backgroundColor: color.surface,
    borderWidth: 1,
    borderColor: color.line,
  },
  budgetNote: { fontSize: 12, lineHeight: 17, color: color.muted },

  tierBlock: { gap: space.sm },
  rows: { gap: space.sm },
  row: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingVertical: space.md,
    paddingHorizontal: space.md,
    borderRadius: radius.md,
    borderWidth: 1,
    borderColor: color.line,
    backgroundColor: color.surface,
  },
  rowPressed: { backgroundColor: color.ground },
  rowName: { fontSize: 16, color: color.ink },
  rowHint: { fontSize: 12, color: color.faint },

  backdrop: { flex: 1, backgroundColor: 'rgba(0,0,0,0.35)' },
  sheet: {
    backgroundColor: color.surface,
    padding: space.xl,
    paddingBottom: 40,
    borderTopLeftRadius: radius.lg,
    borderTopRightRadius: radius.lg,
    gap: space.md,
  },
  sheetName: { fontSize: 20, fontWeight: '600', color: color.ink },
  sheetChip: { flexDirection: 'row' },
  sheetTiers: { flexDirection: 'row', gap: space.sm, flexWrap: 'wrap' },
  sheetFooter: { flexDirection: 'row', gap: space.sm, marginTop: space.sm },
  sheetNote: { fontSize: 12, color: color.faint, lineHeight: 17 },
});
