# Bugs

- Light Theme: The Stdout/Stderr in Light Theme show black font on black background and it's not possible to read
- When tasks start in windows, a black CMD opens but it does not show the name of the argument that is being executed, only something like "python": I propose we make the terminal not appear (always hidden)
- Task Cancel shows 2 cancels
- When we collapse the sidebar it's not possible to open it again and the sidebar at the bottom shows the user misaligned

# Requests

- Include a "Dispatched" run status when it was sent to the worker, but the worker did not start it yet.
- Change the default "max start delay" to 1 minute. A task may start up to X seconds configure in schedule, if it does not start (stays in dispatched) it changes to "Start Failure". In these cases we can make the error code be 5 (make this a system error code)
- Change the timeout to "Max task execution time" inside task and make the change - when a task is in running for longer than this time, the task is cancelled and it gets an status code 6 and execution status "Timeout"
- Remove Neon Theme
- Include an option to run task manually from the Task View
- Include a way of clicking a task version from version >= 2 and get a diff with the previous version
- Each line start in the worker terminal should include something like a > to make it easier to find the start of blocks and it should also show the Task Name and version BEFORE the task ID
- We can't include resources in tasks, there should be a table like Tags inside tasks were we can include resources from the resource list and it shows which resources are exclusive 