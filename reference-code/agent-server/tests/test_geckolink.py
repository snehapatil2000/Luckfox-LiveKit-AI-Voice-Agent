import copy

import pytest

from geckolink import GeckoLinkClient, GeckoLinkError


class FakeGateway:
    def __init__(self):
        self.calls = []
        self.responses = {
            "/api/v1/rooms": {
                "data": {
                    "rooms": [
                        {"id": 1, "name": "Living Room"},
                        {"id": 2, "name": "Kitchen"},
                        {"id": 3, "name": "Hall"},
                    ]
                }
            },
            "/api/v1/points": {
                "data": {
                    "points": [
                        {
                            "id": "aa:01",
                            "name": "Ceiling Light",
                            "room_id": 1,
                            "enabled": True,
                            "capabilities": {"on_off": True, "brightness": True},
                            "state": {"available": True, "on": False, "brightness": 40},
                        },
                        {
                            "id": "bb:01",
                            "name": "Ceiling Light",
                            "room_id": 2,
                            "enabled": True,
                            "capabilities": {"on_off": True},
                            "state": {"available": True, "on": True},
                        },
                        {
                            "id": "cc:01",
                            "name": "Desk Lamp",
                            "room_id": 1,
                            "enabled": True,
                            "capabilities": {"on_off": True},
                            "state": {"available": False, "on": False},
                        },
                        {
                            "id": "sw3:1",
                            "name": "Sw3 - 1",
                            "device_name": "Sw3",
                            "room_id": 3,
                            "enabled": True,
                            "capabilities": {"on_off": True},
                            "state": {"available": True, "on": False},
                        },
                        {
                            "id": "fan:3",
                            "name": "Fan",
                            "device_name": "Sw2",
                            "room_id": 3,
                            "enabled": True,
                            "capabilities": ["on_off", "fan_speed"],
                            "state": {"available": True, "on": True, "fan_speed": 2},
                        },
                    ]
                }
            },
            "/api/v1/scenes": {"data": {"scenes": [{"id": 7, "name": "Movie Night"}]}},
            "/api/v1/moods": {"data": {"moods": [{"id": 9, "name": "Relax"}]}},
        }

    async def request(self, method, path, body):
        self.calls.append((method, path, body))
        return copy.deepcopy(self.responses.get(path, {"data": {}}))


async def test_controls_exact_point_in_requested_room() -> None:
    gateway = FakeGateway()
    client = GeckoLinkClient(gateway.request)

    result = await client.control_point("ceiling light", "off", "kitchen")

    assert result == "Ceiling Light in Kitchen was turned off."
    assert gateway.calls[-1] == ("POST", "/api/v1/points/bb:01/off", {})


async def test_controls_spoken_multigang_switch_name() -> None:
    gateway = FakeGateway()
    client = GeckoLinkClient(gateway.request)

    result = await client.control_point("switch 3 one", "on", "hall")

    assert result == "Sw3 - 1 in Hall was turned on."
    assert gateway.calls[-1] == ("POST", "/api/v1/points/sw3:1/on", {})


async def test_rejects_ambiguous_point_without_sending_control() -> None:
    gateway = FakeGateway()
    client = GeckoLinkClient(gateway.request)

    with pytest.raises(GeckoLinkError, match="More than one point"):
        await client.control_point("ceiling light", "on")

    assert not any(call[0] == "POST" for call in gateway.calls)


async def test_rejects_unavailable_point() -> None:
    gateway = FakeGateway()
    client = GeckoLinkClient(gateway.request)

    with pytest.raises(GeckoLinkError, match="currently unavailable"):
        await client.control_point("desk lamp", "on", "living room")


async def test_sets_brightness_from_zero_to_one_hundred() -> None:
    gateway = FakeGateway()
    client = GeckoLinkClient(gateway.request)

    result = await client.control_point(
        "ceiling light", "set_brightness", "living room", 65
    )

    assert result == "Ceiling Light in Living Room was set to 65 percent brightness."
    assert gateway.calls[-1] == (
        "POST",
        "/api/v1/points/aa:01/set",
        {"brightness": 65},
    )


@pytest.mark.parametrize(
    ("action", "speed", "endpoint", "body", "spoken"),
    [
        ("increase_fan_speed", None, "up", {}, "increased by one level"),
        ("decrease_fan_speed", None, "down", {}, "decreased by one level"),
        ("set_fan_speed", 3, "set", {"speed": 3}, "set to fan speed 3"),
    ],
)
async def test_controls_fan_speed(action, speed, endpoint, body, spoken) -> None:
    gateway = FakeGateway()
    client = GeckoLinkClient(gateway.request)

    result = await client.control_point("fan", action, "hall", fan_speed=speed)

    assert result == f"Fan in Hall was {spoken}."
    assert gateway.calls[-1] == (
        "POST",
        f"/api/v1/points/fan:3/fan/speed/{endpoint}",
        body,
    )


async def test_fan_action_resolves_device_name_to_its_fan_point() -> None:
    gateway = FakeGateway()
    client = GeckoLinkClient(gateway.request)

    result = await client.control_point("sw2", "decrease_fan_speed", "hall")

    assert result == "Fan in Hall was decreased by one level."
    assert gateway.calls[-1] == (
        "POST",
        "/api/v1/points/fan:3/fan/speed/down",
        {},
    )


async def test_reports_live_point_status() -> None:
    client = GeckoLinkClient(FakeGateway().request)

    result = await client.point_status("ceiling light", "living room")

    assert result == "Ceiling Light is off at 40 percent brightness."


async def test_activates_existing_scene_and_mood() -> None:
    gateway = FakeGateway()
    client = GeckoLinkClient(gateway.request)

    assert (
        await client.activate_scene("movie night")
        == "The Movie Night scene was activated."
    )
    assert (
        await client.activate_mood("relax") == "The Relax mood activation was started."
    )
    assert ("POST", "/api/v1/scenes/7/activate", {}) in gateway.calls
    assert ("POST", "/api/v1/moods/9/activate", {}) in gateway.calls
