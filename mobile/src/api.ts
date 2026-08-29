// The GraphQL wire. Kept deliberately small - swap it for a real client if you
// prefer, that choice is yours and we would like to hear the reasoning.
//
// iOS simulator and web reach the API on localhost. A physical device does not:
// set EXPO_PUBLIC_API_URL to http://<your-lan-ip>:8080/query instead.
const API_URL = process.env.EXPO_PUBLIC_API_URL ?? 'http://localhost:8080/query';

export class GraphQLError extends Error {}

export async function gql<T>(
  query: string,
  variables: Record<string, unknown> = {},
  userId?: string,
): Promise<T> {
  const res = await fetch(API_URL, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(userId ? { 'X-User-Id': userId } : {}),
    },
    body: JSON.stringify({ query, variables }),
  });

  if (!res.ok) throw new GraphQLError(`HTTP ${res.status}`);

  const body = (await res.json()) as { data?: T; errors?: { message: string }[] };
  if (body.errors?.length) throw new GraphQLError(body.errors[0].message);
  if (!body.data) throw new GraphQLError('no data');
  return body.data;
}
