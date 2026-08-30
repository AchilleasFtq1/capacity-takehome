import { useCallback, useEffect, useRef, useState } from 'react';

import { gql, GraphQLError } from './api';
import {
  ACCEPT_REQUEST,
  DECLINE_REQUEST,
  MOVE_CONTACT,
  REMOVE_CONTACT,
  SEND_REQUEST,
  SNAPSHOT,
  type Contact,
  type Snapshot,
} from './queries';
import type { TierName } from './theme';

export type Notice = { kind: 'refusal' | 'fault' | 'ok'; text: string };

/** Which rows are mid-flight, keyed by "<action>:<id>". */
export type Busy = Readonly<Record<string, boolean>>;

type State = {
  snapshot: Snapshot | null;
  loading: boolean;
  /** Set only when there is nothing on screen to fall back to. */
  fatal: string | null;
};

export function useAppData(userId: string) {
  const [state, setState] = useState<State>({ snapshot: null, loading: true, fatal: null });
  const [notice, setNotice] = useState<Notice | null>(null);
  const [busy, setBusy] = useState<Busy>({});

  // generation guards against a reply for the previous user landing after a
  // switch and painting their contacts over the current one's.
  const generation = useRef(0);
  // snapshotRef mirrors state.snapshot so callbacks can read the latest value
  // without taking it as a dependency and churning their identity every render.
  const snapshotRef = useRef<Snapshot | null>(null);

  const apply = useCallback((snapshot: Snapshot | null, fatal: string | null) => {
    snapshotRef.current = snapshot;
    setState({ snapshot, loading: false, fatal });
  }, []);

  const refresh = useCallback(async () => {
    const mine = ++generation.current;
    try {
      const data = await gql<Snapshot>(SNAPSHOT, {}, userId);
      if (generation.current !== mine) return;
      apply(data, null);
    } catch (e) {
      if (generation.current !== mine) return;
      const message = e instanceof Error ? e.message : String(e);
      // A reload that fails while something is already on screen is a notice,
      // not a blank page: keep the stale list and say that it is stale.
      if (snapshotRef.current) setNotice({ kind: 'fault', text: message });
      else apply(null, message);
    }
  }, [userId, apply]);

  useEffect(() => {
    snapshotRef.current = null;
    setState({ snapshot: null, loading: true, fatal: null });
    setNotice(null);
    void refresh();
  }, [userId, refresh]);

  /**
   * Runs a mutation and turns the outcome into a notice.
   *
   * Refusals are the interesting case: the server has already written the
   * sentence, with the numbers in it, so the UI shows it verbatim rather than
   * inventing its own wording from an error code.
   */
  const run = useCallback(async (key: string, work: () => Promise<string>) => {
    setBusy((b) => ({ ...b, [key]: true }));
    setNotice(null);
    try {
      setNotice({ kind: 'ok', text: await work() });
      return true;
    } catch (e) {
      const refused = e instanceof GraphQLError && e.isRefusal;
      setNotice({
        kind: refused ? 'refusal' : 'fault',
        text: e instanceof Error ? e.message : String(e),
      });
      return false;
    } finally {
      setBusy((b) => ({ ...b, [key]: false }));
    }
  }, []);

  const sendRequest = useCallback(
    (toUserId: string, tier: TierName, name: string) =>
      run(`send:${toUserId}`, async () => {
        await gql(SEND_REQUEST, { toUserId, tier }, userId);
        await refresh();
        return `Request sent to ${name}.`;
      }),
    [run, refresh, userId],
  );

  /**
   * R8 - optimistic accept with rollback.
   *
   * The contact appears the instant it is tapped, because that is almost always
   * what is about to happen. When the server refuses - the last seat went to
   * someone else, or the sender filled up while the request sat in the inbox -
   * the whole previous snapshot goes back exactly as it was and the refusal is
   * shown. Restoring a captured snapshot rather than reversing each individual
   * edit is what keeps the rollback correct when a refresh lands mid-flight.
   */
  const acceptRequest = useCallback(
    (requestId: string) => {
      const before = snapshotRef.current;
      const request = before?.incomingRequests.find((r) => r.id === requestId);

      if (before && request) {
        apply(
          {
            ...before,
            contacts: [
              ...before.contacts,
              {
                id: `optimistic:${requestId}`,
                user: request.from,
                tier: request.tier,
                createdAt: new Date().toISOString(),
              } satisfies Contact,
            ],
            incomingRequests: before.incomingRequests.filter((r) => r.id !== requestId),
            capacity: {
              ...before.capacity,
              budgetUsed: before.capacity.budgetUsed + 1,
              tiers: before.capacity.tiers.map((t) =>
                t.tier === request.tier ? { ...t, used: t.used + 1 } : t,
              ),
            },
          },
          null,
        );
      }

      return run(`accept:${requestId}`, async () => {
        try {
          await gql(ACCEPT_REQUEST, { requestId }, userId);
        } catch (e) {
          if (before) apply(before, null);
          throw e;
        }
        await refresh();
        return request ? `${request.from.name} is now a contact.` : 'Request accepted.';
      });
    },
    [run, refresh, userId, apply],
  );

  const declineRequest = useCallback(
    (requestId: string, name: string) =>
      run(`decline:${requestId}`, async () => {
        await gql(DECLINE_REQUEST, { requestId }, userId);
        await refresh();
        return `Declined ${name}.`;
      }),
    [run, refresh, userId],
  );

  const moveContact = useCallback(
    (contactId: string, tier: TierName, name: string) =>
      run(`move:${contactId}`, async () => {
        await gql(MOVE_CONTACT, { contactId, tier }, userId);
        await refresh();
        return `Moved ${name}.`;
      }),
    [run, refresh, userId],
  );

  const removeContact = useCallback(
    (contactId: string, name: string) =>
      run(`remove:${contactId}`, async () => {
        await gql(REMOVE_CONTACT, { contactId }, userId);
        await refresh();
        return `Removed ${name}. The seat is free on both sides.`;
      }),
    [run, refresh, userId],
  );

  const dismissNotice = useCallback(() => setNotice(null), []);

  return {
    ...state,
    notice,
    busy,
    dismissNotice,
    refresh,
    sendRequest,
    acceptRequest,
    declineRequest,
    moveContact,
    removeContact,
  };
}
