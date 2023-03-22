def process_file(file_path):
    with open(file_path, 'r') as file:
        lines = file.readlines()

    node_count = 0
    node_lines_count = 0
    batch_count = 0
    batch_lines_count = 0
    consecutive_blank_lines = 0

    for line in lines:
        if line.strip() == '':
            consecutive_blank_lines += 1
            if consecutive_blank_lines == 1:
                node_count += 1
            elif consecutive_blank_lines >= 3:
                batch_count += 1
                consecutive_blank_lines = 0
        else:
            node_lines_count += 1
            batch_lines_count += 1
            consecutive_blank_lines = 0

    node_count += 1
    batch_count += 1

    average_lines_per_node = node_lines_count / node_count
    average_lines_per_batch = batch_lines_count / batch_count

    return {
        'node_count': node_count,
        'average_lines_per_node': average_lines_per_node,
        'batch_count': batch_count,
        'average_lines_per_batch': average_lines_per_batch,
    }

if __name__ == "__main__":
    file_path = '/data/trieHashimoto/th_experiment_data_3/logs/batch16/0316_setHead_100000_depth+[0:9]_push.txt'  # Replace with the path to your text file

    result = process_file(file_path)

    print(f"Total Number of Nodes: {result['node_count']}")
    print(f"Average number of childs per node: {result['average_lines_per_node']}")
    print(f"Total Number of Batches: {result['batch_count']}")
    print(f"Average number of nodes per batch: {result['average_lines_per_batch']}")