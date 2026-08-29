import { useCallback, useEffect, useState } from 'react';
import {
  ActivityIndicator,
  Pressable,
  SafeAreaView,
  ScrollView,
  StyleSheet,
  Text,
  View,
} from 'react-native';
import { StatusBar } from 'expo-status-bar';

import { gql } from './src/api';

type User = { id: string; name: string };

/**
 * This screen exists to prove the wire works and to let you switch who you are.
 * Everything the brief asks for (R5 the people list, R6 the request inbox) is
 * yours to build - replace or restructure any of this.
 */
export default function App() {
  const [users, setUsers] = useState<User[]>([]);
  const [me, setMe] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    try {
      setError(null);
      const data = await gql<{ users: User[] }>(`{ users { id name } }`);
      setUsers(data.users);
      setMe((current) => current ?? data.users[0]?.id ?? null);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  return (
    <SafeAreaView style={styles.screen}>
      <StatusBar style="auto" />
      <ScrollView contentContainerStyle={styles.body}>
        <Text style={styles.title}>Acting as</Text>

        {loading && <ActivityIndicator />}

        {error && (
          <View style={styles.error}>
            <Text style={styles.errorText}>{error}</Text>
            <Text style={styles.hint}>
              Is the API up? `make up` then `make api`, then pull to retry.
            </Text>
            <Pressable onPress={load} style={styles.retry}>
              <Text style={styles.retryText}>Retry</Text>
            </Pressable>
          </View>
        )}

        <View style={styles.list}>
          {users.map((u) => {
            const active = u.id === me;
            return (
              <Pressable
                key={u.id}
                onPress={() => setMe(u.id)}
                style={[styles.row, active && styles.rowActive]}
              >
                <Text style={[styles.name, active && styles.nameActive]}>{u.name}</Text>
                {active && <Text style={styles.check}>current</Text>}
              </Pressable>
            );
          })}
        </View>

        <Text style={styles.todo}>
          Build from here: contacts grouped by tier with live used/cap counts (R5),
          and the request inbox (R6). Pass `me` as the userId argument to gql().
        </Text>
      </ScrollView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: '#fff' },
  body: { padding: 20, gap: 16 },
  title: { fontSize: 13, textTransform: 'uppercase', letterSpacing: 1, color: '#888' },
  list: { gap: 8 },
  row: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: 14,
    borderRadius: 10,
    borderWidth: 1,
    borderColor: '#e3e3e3',
  },
  rowActive: { borderColor: '#16605c', backgroundColor: '#e1efed' },
  name: { fontSize: 16, color: '#222' },
  nameActive: { fontWeight: '600', color: '#16605c' },
  check: { fontSize: 11, color: '#16605c', textTransform: 'uppercase', letterSpacing: 1 },
  error: { padding: 14, borderRadius: 10, backgroundColor: '#f6e7e4', gap: 8 },
  errorText: { color: '#9a3b2e', fontSize: 14 },
  hint: { color: '#9a3b2e', fontSize: 12, opacity: 0.8 },
  retry: { alignSelf: 'flex-start', paddingVertical: 6, paddingHorizontal: 12, borderRadius: 6, backgroundColor: '#9a3b2e' },
  retryText: { color: '#fff', fontSize: 13 },
  todo: { marginTop: 12, fontSize: 13, lineHeight: 20, color: '#888' },
});
