import useConfig from "../useConfig";

export default function ChannelSelector({
  selected,
  onTune,
}: {
  selected?: string;
  onTune: (ch: string) => void;
}) {
  const channelNames = useConfig<string[]>("channels");

  return Array.isArray(channelNames) ? (
    <aside className="ChannelSelector">
      {channelNames.map((ch) => (
        <Channel
          key={ch}
          name={ch}
          active={ch === selected}
          onClick={() => onTune(ch)}
        />
      ))}
    </aside>
  ) : null;
}

function Channel({
  name,
  active,
  onClick,
}: {
  name: string;
  active?: boolean;
  onClick: () => void;
}) {
  return (
    <button
      className={`ChannelSelector__Channel ${
        active ? "ChannelSelector__Channel--Active" : ""
      }`}
      onClick={onClick}
    >
      {name}
    </button>
  );
}
