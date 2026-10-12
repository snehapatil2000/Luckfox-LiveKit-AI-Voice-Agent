"""Small, deterministic GeckoLink client used by the voice-agent tools."""

import re
from collections.abc import Awaitable, Callable
from typing import Any, Literal

Request = Callable[[str, str, dict[str, Any] | None], Awaitable[dict[str, Any]]]
PointAction = Literal[
    "on",
    "off",
    "toggle",
    "set_brightness",
    "increase_fan_speed",
    "decrease_fan_speed",
    "set_fan_speed",
]
NUMBER_WORDS = {
    "zero": "0",
    "one": "1",
    "two": "2",
    "three": "3",
    "four": "4",
    "five": "5",
    "six": "6",
    "seven": "7",
    "eight": "8",
    "nine": "9",
}


class GeckoLinkError(RuntimeError):
    """A safe error that can be spoken to the user."""


def _normalize(value: Any) -> str:
    text = str(value or "").casefold()
    text = " ".join("".join(char if char.isalnum() else " " for char in text).split())
    number = r"\d+|" + "|".join(NUMBER_WORDS)
    text = re.sub(
        rf"\b(?:switch|s\s+w)\s+({number})\b",
        lambda match: "sw" + NUMBER_WORDS.get(match.group(1), match.group(1)),
        text,
    )
    return re.sub(
        rf"\b(sw\d+)\s+({number})\b",
        lambda match: f"{match.group(1)} {NUMBER_WORDS.get(match.group(2), match.group(2))}",
        text,
    )


def _supports(capabilities: Any, name: str) -> bool:
    if isinstance(capabilities, dict):
        return bool(capabilities.get(name, False))
    if isinstance(capabilities, list):
        return name in capabilities
    return True


def _point_supports(point: dict[str, Any], name: str) -> bool:
    """Use live state when older gateways omit a capability declaration."""
    state = point.get("state")
    if isinstance(state, dict) and name in state:
        return True
    return _supports(point.get("capabilities"), name)


class GeckoLinkClient:
    """Resolve human names, then call a restricted GeckoLink API transport."""

    def __init__(self, request: Request):
        self._request = request

    async def _list(self, path: str, key: str) -> list[dict[str, Any]]:
        response = await self._request("GET", path, None)
        data = response.get("data", response)
        values = data.get(key) if isinstance(data, dict) else None
        if not isinstance(values, list):
            raise GeckoLinkError(f"The gateway returned an invalid {key} list.")
        return [value for value in values if isinstance(value, dict)]

    async def _points_with_rooms(self) -> list[dict[str, Any]]:
        points, rooms = (
            await self._list("/api/v1/points", "points"),
            await self._list("/api/v1/rooms", "rooms"),
        )
        room_names = {
            str(room.get("id")): str(room.get("name") or "") for room in rooms
        }
        for point in points:
            point["room_name"] = room_names.get(str(point.get("room_id")), "")
        return points

    @staticmethod
    def _match_named(
        items: list[dict[str, Any]], name: str, kind: str
    ) -> dict[str, Any]:
        wanted = _normalize(name)
        if not wanted:
            raise GeckoLinkError(f"Please say which {kind} you want.")

        exact = [
            item
            for item in items
            if wanted in {_normalize(item.get("id")), _normalize(item.get("name"))}
        ]
        matches = exact or [
            item
            for item in items
            if wanted in _normalize(item.get("name"))
            or _normalize(item.get("name")) in wanted
        ]
        if not matches:
            raise GeckoLinkError(f"I could not find a {kind} named {name}.")
        if len(matches) > 1:
            choices = ", ".join(
                str(item.get("name") or item.get("id")) for item in matches[:4]
            )
            raise GeckoLinkError(f"More than one {kind} matches {name}: {choices}.")
        return matches[0]

    async def _resolve_point(
        self, name: str, room: str = "", capability: str = ""
    ) -> dict[str, Any]:
        points = await self._points_with_rooms()
        wanted_room = _normalize(room)
        if wanted_room:
            points = [
                point
                for point in points
                if wanted_room == _normalize(point.get("room_name"))
                or wanted_room in _normalize(point.get("room_name"))
            ]
            if not points:
                raise GeckoLinkError(f"I could not find any points in {room}.")

        wanted = _normalize(name)
        exact = [
            point
            for point in points
            if wanted
            in {
                _normalize(point.get("id")),
                _normalize(point.get("name")),
                _normalize(point.get("device_name")),
            }
        ]
        matches = exact or [
            point
            for point in points
            if wanted
            and (
                wanted in _normalize(point.get("name"))
                or wanted in _normalize(point.get("device_name"))
            )
        ]
        if len(matches) > 1 and capability:
            capable_matches = [
                point
                for point in matches
                if _point_supports(point, capability)
            ]
            if capable_matches:
                matches = capable_matches
        if not matches:
            location = f" in {room}" if room else ""
            raise GeckoLinkError(f"I could not find a point named {name}{location}.")
        if len(matches) > 1:
            choices = ", ".join(
                f"{point.get('name')} in {point.get('room_name') or 'no room'}"
                for point in matches[:4]
            )
            raise GeckoLinkError(f"More than one point matches {name}: {choices}.")
        return matches[0]

    async def find_points(self, query: str = "", room: str = "") -> str:
        points = await self._points_with_rooms()
        wanted_query, wanted_room = _normalize(query), _normalize(room)
        matches = [
            point
            for point in points
            if (not wanted_room or wanted_room in _normalize(point.get("room_name")))
            and (
                not wanted_query
                or wanted_query in _normalize(point.get("name"))
                or wanted_query in _normalize(point.get("device_name"))
            )
        ]
        if not matches:
            return "No matching automation points were found."
        descriptions = [
            f"{point.get('name') or point.get('id')} in "
            f"{point.get('room_name') or 'no assigned room'}"
            for point in matches[:10]
        ]
        suffix = " Additional matches were omitted." if len(matches) > 10 else ""
        return "Available points: " + "; ".join(descriptions) + "." + suffix

    async def point_status(self, name: str, room: str = "") -> str:
        point = await self._resolve_point(name, room)
        state = point.get("state") if isinstance(point.get("state"), dict) else {}
        available = point.get("available", state.get("available", True))
        enabled = point.get("enabled", True)
        if not enabled:
            status = "disabled"
        elif not available:
            status = "unavailable"
        elif "on" in state:
            status = "on" if state["on"] else "off"
        elif "on_off" in state:
            status = "on" if state["on_off"] else "off"
        else:
            status = "available"
        brightness = state.get("brightness")
        brightness_text = (
            f" at {brightness} percent brightness" if brightness is not None else ""
        )
        fan_speed = state.get("fan_speed")
        fan_speed_text = (
            f" at fan speed {fan_speed}" if fan_speed is not None else ""
        )
        return f"{point.get('name')} is {status}{brightness_text}{fan_speed_text}."

    async def control_point(
        self,
        name: str,
        action: PointAction,
        room: str = "",
        brightness: int | None = None,
        fan_speed: int | None = None,
    ) -> str:
        fan_actions = {
            "increase_fan_speed",
            "decrease_fan_speed",
            "set_fan_speed",
        }
        point = await self._resolve_point(
            name, room, "fan_speed" if action in fan_actions else ""
        )
        state = point.get("state") if isinstance(point.get("state"), dict) else {}
        if not point.get("enabled", True):
            raise GeckoLinkError(f"{point.get('name')} is disabled.")
        if not point.get("available", state.get("available", True)):
            raise GeckoLinkError(f"{point.get('name')} is currently unavailable.")

        capabilities = point.get("capabilities")
        endpoint: str
        body: dict[str, Any]
        if action in fan_actions:
            if capabilities and not _point_supports(point, "fan_speed"):
                raise GeckoLinkError(f"{point.get('name')} does not support fan speed.")
            if action == "set_fan_speed":
                if fan_speed is None or not 1 <= fan_speed <= 4:
                    raise GeckoLinkError("Fan speed must be between 1 and 4.")
                endpoint, body = "fan/speed/set", {"speed": fan_speed}
                spoken_action = f"set to fan speed {fan_speed}"
            else:
                direction = "up" if action == "increase_fan_speed" else "down"
                endpoint, body = f"fan/speed/{direction}", {}
                spoken_action = (
                    "increased by one level"
                    if direction == "up"
                    else "decreased by one level"
                )
        elif action == "set_brightness":
            if brightness is None or not 0 <= brightness <= 100:
                raise GeckoLinkError("Brightness must be between 0 and 100 percent.")
            if capabilities and not _point_supports(point, "brightness"):
                raise GeckoLinkError(
                    f"{point.get('name')} does not support brightness."
                )
            endpoint, body = "set", {"brightness": brightness}
            spoken_action = f"set to {brightness} percent brightness"
        else:
            if capabilities and not _supports(capabilities, "on_off"):
                raise GeckoLinkError(
                    f"{point.get('name')} does not support on and off control."
                )
            endpoint, body = action, {}
            spoken_action = {
                "on": "turned on",
                "off": "turned off",
                "toggle": "toggled",
            }[action]

        await self._request("POST", f"/api/v1/points/{point['id']}/{endpoint}", body)
        room_name = point.get("room_name")
        location = f" in {room_name}" if room_name else ""
        return f"{point.get('name')}{location} was {spoken_action}."

    async def activate_scene(self, name: str) -> str:
        scene = self._match_named(
            await self._list("/api/v1/scenes", "scenes"), name, "scene"
        )
        await self._request("POST", f"/api/v1/scenes/{scene['id']}/activate", {})
        return f"The {scene.get('name')} scene was activated."

    async def activate_mood(self, name: str) -> str:
        mood = self._match_named(
            await self._list("/api/v1/moods", "moods"), name, "mood"
        )
        await self._request("POST", f"/api/v1/moods/{mood['id']}/activate", {})
        return f"The {mood.get('name')} mood activation was started."
