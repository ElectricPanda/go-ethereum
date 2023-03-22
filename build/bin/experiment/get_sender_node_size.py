import statistics

filename = "/data/trieHashimoto/th_experiment_data_3/logs/batch1/0318_setHead_100000_depth+[0:9]_sender.txt"

row_count = 0
node_sizes = []

with open(filename, "r") as file:
    for line in file:
        row_count += 1

        try:
            node_size_str = line.split("Node Size:")[-1].strip()
            node_size = int(node_size_str)
            node_sizes.append(node_size)
        except (IndexError, ValueError):
            print(f"Error parsing line {row_count}: {line.strip()}")

if not node_sizes:
    print("No valid node sizes found in the input file.")
else:
    total_node_size = sum(node_sizes)
    avg_node_size = total_node_size / row_count
    stdev_node_size = statistics.stdev(node_sizes)
    max_node_size = max(node_sizes)
    min_node_size = min(node_sizes)

    print(f"Total number of rows: {row_count}")
    print(f"Sum of Node Size values: {total_node_size}")
    print(f"Average Node Size: {avg_node_size:.2f}")
    print(f"Standard Deviation of Node Size: {stdev_node_size:.2f}")
    print(f"Max Node Size: {max_node_size}")
    print(f"Min Node Size: {min_node_size}")