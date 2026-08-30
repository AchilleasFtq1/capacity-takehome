// The GraphQL wire. Kept deliberately small - swap it for a real client if you
// prefer, that choice is yours and we would like to hear the reasoning.
//
// iOS simulator and web reach the API on localhost. A physical device does not:
// set EXPO_PUBLIC_API_URL to http://<your-lan-ip>:8080/query instead.
const API_URL = process.env.EXPO_PUBLIC_API_URL ?? 'http://localhost:8080/query';

/**
 * A GraphQLError carries the sentence the server wants the user to read.
 *
 * The API splits refusals ("Your Pink flag tier is full (1 of 1)") from faults,
 * and only refusals arrive with prose worth showing. `code` is the stable half
 * of that contract, so the UI can style a refusal differently from a crash
 * without ever matching on the wording.
 */
export class GraphQLError extends Error {
  readonly code: string;

  constructor(message: string, code = 'UNKNOWN') {
    super(message);
    this.name = 'GraphQLError';
    this.code = code;
  }

  /** True when the server refused on purpose and explained itself. */
  get isRefusal(): boolean {
    return this.code !== 'UNKNOWN' && this.code !== 'INTERNAL';
  }
}

type GraphQLResponse<T> = {
  data?: T;
  errors?: { message: string; extensions?: { code?: string } }[];
};

export async function gql<T>(
  query: string,
  variables: Record<string, unknown> = {},
  userId?: string,
): Promise<T> {
  let res: Response;
  try {
    res = await fetch(API_URL, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...(userId ? { 'X-User-Id': userId } : {}),
      },
      body: JSON.stringify({ query, variables }),
    });
  } catch {
    // fetch only rejects when the request never landed, which in practice means
    // the API is not running. Say that, rather than "Network request failed".
    throw new GraphQLError(`Can't reach the API at ${API_URL}. Is it running?`, 'OFFLINE');
  }

  // A refused mutation is a 200 with an errors array; only a malformed request
  // or a validation failure is a non-2xx, and those still carry a usable body.
  let body: GraphQLResponse<T>;
  try {
    body = (await res.json()) as GraphQLResponse<T>;
  } catch {
    throw new GraphQLError(`HTTP ${res.status} from the API.`, 'INTERNAL');
  }

  const failure = body.errors?.[0];
  if (failure) {
    throw new GraphQLError(failure.message, failure.extensions?.code ?? 'UNKNOWN');
  }
  if (!body.data) throw new GraphQLError('The API returned no data.', 'INTERNAL');
  return body.data;
}
