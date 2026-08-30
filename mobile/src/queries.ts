import type { TierName } from './theme';

export type User = { id: string; name: string };

export type Contact = {
  id: string;
  user: User;
  tier: TierName;
  createdAt: string;
};

export type TierCapacity = { tier: TierName; used: number; cap: number };

export type Capacity = {
  budgetUsed: number;
  budgetCap: number;
  tiers: TierCapacity[];
};

export type RequestStatus = 'PENDING' | 'ACCEPTED' | 'DECLINED';

export type IncomingRequest = {
  id: string;
  from: User;
  tier: TierName;
  createdAt: string;
};

export type OutgoingRequest = {
  id: string;
  to: User;
  tier: TierName;
  status: RequestStatus;
  createdAt: string;
};

/** Everything one screen-load needs, in a single round trip. */
export type Snapshot = {
  me: User;
  users: User[];
  contacts: Contact[];
  capacity: Capacity;
  incomingRequests: IncomingRequest[];
  outgoingRequests: OutgoingRequest[];
};

export const SNAPSHOT = `
  query Snapshot {
    me { id name }
    users { id name }
    contacts { id tier createdAt user { id name } }
    capacity { budgetUsed budgetCap tiers { tier used cap } }
    incomingRequests { id tier createdAt from { id name } }
    outgoingRequests { id tier status createdAt to { id name } }
  }
`;

export const SEND_REQUEST = `
  mutation SendRequest($toUserId: ID!, $tier: Tier!) {
    sendRequest(toUserId: $toUserId, tier: $tier) { id tier status to { id name } }
  }
`;

export const ACCEPT_REQUEST = `
  mutation AcceptRequest($requestId: ID!) {
    acceptRequest(requestId: $requestId) { id tier createdAt user { id name } }
  }
`;

export const DECLINE_REQUEST = `
  mutation DeclineRequest($requestId: ID!) {
    declineRequest(requestId: $requestId) { id status }
  }
`;

export const MOVE_CONTACT = `
  mutation MoveContact($contactId: ID!, $tier: Tier!) {
    moveContact(contactId: $contactId, tier: $tier) { id tier }
  }
`;

export const REMOVE_CONTACT = `
  mutation RemoveContact($contactId: ID!) {
    removeContact(contactId: $contactId)
  }
`;
