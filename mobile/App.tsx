import { useCallback, useEffect, useState } from 'react';
import {
  ActivityIndicator,
  Modal,
  Pressable,
  RefreshControl,
  SafeAreaView,
  ScrollView,
  StyleSheet,
  Text,
  View,
} from 'react-native';
import { StatusBar } from 'expo-status-bar';

import { gql } from './src/api';
import { Button, NoticeBanner, Segmented } from './src/components';
import type { User } from './src/queries';
import { AddScreen } from './src/screens/Add';
import { PeopleScreen } from './src/screens/People';
import { RequestsScreen } from './src/screens/Requests';
import { color, radius, space } from './src/theme';
import { useAppData } from './src/useAppData';

type Tab = 'people' | 'requests' | 'add';

export default function App() {
  // Who am I acting as. There is no auth in this exercise by design, so this is
  // the whole session: an id that gets sent as X-User-Id.
  const [me, setMe] = useState<User | null>(null);
  const [users, setUsers] = useState<User[]>([]);
  const [bootError, setBootError] = useState<string | null>(null);
  const [booting, setBooting] = useState(true);

  const boot = useCallback(async () => {
    setBooting(true);
    try {
      setBootError(null);
      const data = await gql<{ users: User[] }>(`{ users { id name } }`);
      setUsers(data.users);
      setMe((current) => current ?? data.users[0] ?? null);
    } catch (e) {
      setBootError(e instanceof Error ? e.message : String(e));
    } finally {
      setBooting(false);
    }
  }, []);

  useEffect(() => {
    void boot();
  }, [boot]);

  if (booting) {
    return (
      <SafeAreaView style={styles.screen}>
        <View style={styles.centre}>
          <ActivityIndicator />
        </View>
      </SafeAreaView>
    );
  }

  if (bootError || !me) {
    return (
      <SafeAreaView style={styles.screen}>
        <View style={styles.centre}>
          <Text style={styles.bootTitle}>Can't start</Text>
          <Text style={styles.bootText}>{bootError ?? 'No users came back from the API.'}</Text>
          <Text style={styles.bootHint}>Run `make up` then `make api`, then try again.</Text>
          <Button label="Retry" kind="primary" onPress={() => void boot()} />
        </View>
      </SafeAreaView>
    );
  }

  return <Home me={me} users={users} onSwitch={setMe} />;
}

function Home({
  me,
  users,
  onSwitch,
}: {
  me: User;
  users: User[];
  onSwitch: (u: User) => void;
}) {
  const [tab, setTab] = useState<Tab>('people');
  const [switching, setSwitching] = useState(false);
  const data = useAppData(me.id);

  const snapshot = data.snapshot;
  const inboxCount = snapshot?.incomingRequests.length ?? 0;

  return (
    <SafeAreaView style={styles.screen}>
      <StatusBar style="auto" />

      <View style={styles.header}>
        <Pressable
          onPress={() => setSwitching(true)}
          accessibilityRole="button"
          accessibilityLabel={`Acting as ${me.name}. Switch user.`}
          style={styles.whoami}
        >
          <View>
            <Text style={styles.whoamiLabel}>Acting as</Text>
            <Text style={styles.whoamiName}>{me.name}</Text>
          </View>
          <Text style={styles.whoamiSwitch}>Switch</Text>
        </Pressable>

        <Segmented<Tab>
          value={tab}
          onChange={setTab}
          options={[
            { value: 'people', label: 'People' },
            { value: 'requests', label: 'Requests', badge: inboxCount },
            { value: 'add', label: 'Add' },
          ]}
        />
      </View>

      {data.notice && (
        <View style={styles.noticeWrap}>
          <NoticeBanner notice={data.notice} onDismiss={data.dismissNotice} />
        </View>
      )}

      <ScrollView
        contentContainerStyle={styles.body}
        refreshControl={
          <RefreshControl refreshing={data.loading && !!snapshot} onRefresh={() => void data.refresh()} />
        }
      >
        {data.loading && !snapshot && <ActivityIndicator />}

        {data.fatal && (
          <View style={styles.centre}>
            <Text style={styles.bootTitle}>Can't load</Text>
            <Text style={styles.bootText}>{data.fatal}</Text>
            <Button label="Retry" kind="primary" onPress={() => void data.refresh()} />
          </View>
        )}

        {snapshot && tab === 'people' && (
          <PeopleScreen
            snapshot={snapshot}
            busy={data.busy}
            onMove={data.moveContact}
            onRemove={data.removeContact}
          />
        )}
        {snapshot && tab === 'requests' && (
          <RequestsScreen
            snapshot={snapshot}
            busy={data.busy}
            onAccept={data.acceptRequest}
            onDecline={data.declineRequest}
          />
        )}
        {snapshot && tab === 'add' && (
          <AddScreen snapshot={snapshot} busy={data.busy} onSend={data.sendRequest} />
        )}
      </ScrollView>

      <Modal
        visible={switching}
        transparent
        animationType="slide"
        onRequestClose={() => setSwitching(false)}
      >
        <Pressable
          style={styles.backdrop}
          onPress={() => setSwitching(false)}
          accessibilityLabel="Close"
        />
        <View style={styles.sheet}>
          <Text style={styles.sheetTitle}>Act as</Text>
          <ScrollView style={styles.sheetList}>
            {users.map((u) => {
              const active = u.id === me.id;
              return (
                <Pressable
                  key={u.id}
                  onPress={() => {
                    onSwitch(u);
                    setSwitching(false);
                  }}
                  accessibilityRole="button"
                  accessibilityState={{ selected: active }}
                  style={[styles.switchRow, active && styles.switchRowActive]}
                >
                  <Text style={[styles.switchName, active && styles.switchNameActive]}>{u.name}</Text>
                  {active && <Text style={styles.switchCurrent}>current</Text>}
                </Pressable>
              );
            })}
          </ScrollView>
        </View>
      </Modal>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: color.ground },
  centre: { padding: space.xl, gap: space.md, alignItems: 'flex-start' },
  bootTitle: { fontSize: 18, fontWeight: '600', color: color.ink },
  bootText: { fontSize: 14, color: color.danger, lineHeight: 20 },
  bootHint: { fontSize: 12, color: color.muted },

  header: {
    paddingHorizontal: space.lg,
    paddingTop: space.md,
    paddingBottom: space.md,
    gap: space.md,
    backgroundColor: color.surface,
    borderBottomWidth: 1,
    borderBottomColor: color.line,
  },
  whoami: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between' },
  whoamiLabel: {
    fontSize: 11,
    textTransform: 'uppercase',
    letterSpacing: 1,
    color: color.faint,
    fontWeight: '600',
  },
  whoamiName: { fontSize: 22, fontWeight: '600', color: color.ink },
  whoamiSwitch: { fontSize: 14, color: color.accent, fontWeight: '500' },

  noticeWrap: { paddingHorizontal: space.lg, paddingTop: space.md },
  body: { padding: space.lg, paddingBottom: 48, gap: space.lg },

  backdrop: { flex: 1, backgroundColor: 'rgba(0,0,0,0.35)' },
  sheet: {
    maxHeight: '70%',
    backgroundColor: color.surface,
    padding: space.xl,
    paddingBottom: 40,
    borderTopLeftRadius: radius.lg,
    borderTopRightRadius: radius.lg,
    gap: space.md,
  },
  sheetTitle: { fontSize: 18, fontWeight: '600', color: color.ink },
  sheetList: { flexGrow: 0 },
  switchRow: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: space.md,
    borderRadius: radius.md,
    borderWidth: 1,
    borderColor: color.line,
    marginBottom: space.sm,
  },
  switchRowActive: { borderColor: color.accent, backgroundColor: color.accentWash },
  switchName: { fontSize: 16, color: color.ink },
  switchNameActive: { fontWeight: '600', color: color.accent },
  switchCurrent: {
    fontSize: 11,
    color: color.accent,
    textTransform: 'uppercase',
    letterSpacing: 1,
  },
});
