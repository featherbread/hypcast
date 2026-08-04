import React from "react";

export default function useConfig<T>(name: string): undefined | T | Error {
  const [result, setResult] = React.useState<undefined | T | Error>();

  React.useEffect(() => {
    startFetch().catch(() => {});

    async function startFetch() {
      setResult(undefined);
      try {
        // TODO: This is legitimately an unsafe type assertion, which trusts the
        // user of this hook to ensure total alignment between the server and
        // client. This isn't a serious issue in the current version of Hypcast,
        // where clients won't reconnect to the server without a page refresh,
        // but could become one if I ever do auto-reconnection.
        // eslint-disable-next-line typescript/no-unsafe-type-assertion
        setResult((await fetchConfigWithCache(name)) as T);
      } catch (e: unknown) {
        if (e instanceof Error) {
          setResult(e);
        } else {
          // We're just going to hope the error can be stringified.
          // eslint-disable-next-line typescript/restrict-template-expressions
          setResult(Error(`${e}`));
        }
      }
    }
  }, [name]);

  return result;
}

const FETCH_CACHE = new Map<string, Promise<unknown>>();

function fetchConfigWithCache(name: string): Promise<unknown> {
  const cachedPromise = FETCH_CACHE.get(name);
  if (cachedPromise !== undefined) {
    return cachedPromise;
  }

  const responsePromise = fetch(`/api/config/${name}`).then((response) => {
    if (!response.ok) {
      return new Error(
        `failed to retrieve ${name} config: ${response.statusText}`,
      );
    }
    return response.json();
  });

  FETCH_CACHE.set(name, responsePromise);
  return responsePromise;
}
