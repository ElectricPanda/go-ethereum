import re
import matplotlib
matplotlib.use('Agg')  # Set the backend to Agg, which can save plots as image files
import matplotlib.pyplot as plt
import statistics

cleanCacheHitTotalTime = 0
cleanCacheHitCount = 0
levelDbCacheHitTotalTime = 0
levelDbCacheHitCount = 0
levelDbCacheMissTotalTime = 0
levelDbCacheMissCount = 0
hash_values = []
row_count = 0
node_sizes = []

with open('file.txt') as f:
    for line in f:
        time_str = line.split('Took time: ')[1].split()[0]
        time = int(time_str)

        if 'CleanCacheHitData: true' in line:
            cleanCacheHitTotalTime += time
            cleanCacheHitCount += 1
        elif 'LevelDBCacheMiss: true' in line:
            levelDbCacheMissTotalTime += time
            levelDbCacheMissCount += 1
        else:
            levelDbCacheHitTotalTime += time
            levelDbCacheHitCount += 1
        
        match = re.search(r'Hash: (0x[0-9a-fA-F]{64})', line)
        if match:
            # Normalize the hash values
            hash_value = int(match.group(1), 16) / (2**256 - 1)
            hash_values.append(hash_value)
        
        row_count += 1

        node_size_str = line.split("Node Size:")[-1].strip()
        node_size = int(node_size_str)
        node_sizes.append(node_size)

if cleanCacheHitCount == 0:
    print("No rows with CleanCacheHitData = true")
else:
    cleanCacheHitAvgTime = cleanCacheHitTotalTime / cleanCacheHitCount
    print(f"Total Retrieve Time: {cleanCacheHitTotalTime+levelDbCacheMissTotalTime+levelDbCacheHitTotalTime}")
    print(f"Total cleanCacheHitTotalTime: {cleanCacheHitTotalTime}")
    print(f"Total CleanCacheHit cleanCacheHitCount: {cleanCacheHitCount}")
    print(f"Average CleanCacheHit Retrieve time: {cleanCacheHitAvgTime}")
if levelDbCacheHitCount == 0:
    print("No rows with LevelDBCacheHit")
else:
    levelDbCacheHitAvgTime = levelDbCacheHitTotalTime / levelDbCacheHitCount
    print(f"Total Retrieve Time: {cleanCacheHitTotalTime+levelDbCacheMissTotalTime+levelDbCacheHitTotalTime}")
    print(f"Total levelDbCacheHitTotalTime: {levelDbCacheHitTotalTime}")
    print(f"Total LevelDBCacheHit levelDbCacheHitCount: {levelDbCacheHitCount}")
    print(f"Average LevelDBCacheHit Retrieve time: {levelDbCacheHitAvgTime}")

if levelDbCacheMissCount == 0:
    print("No rows with LevelDBCacheMiss")
else:
    levelDbCacheMissAvgTime = levelDbCacheMissTotalTime / levelDbCacheMissCount
    print(f"Total Retrieve Time: {cleanCacheHitTotalTime+levelDbCacheMissTotalTime+levelDbCacheHitTotalTime}")
    print(f"Total levelDbCacheMissTotalTime: {levelDbCacheMissTotalTime}")
    print(f"Total LevelDBCacheMiss levelDbCacheMissCount: {levelDbCacheMissCount}")
    print(f"Average LevelDBCacheMiss Retrieve time: {levelDbCacheMissAvgTime}")

# Plot a histogram of the hash values
plt.hist(hash_values, bins='auto', edgecolor='black')
plt.xlabel('Normalized Hash Value Range')
plt.ylabel('Frequency')
plt.title('Histogram of Normalized Hash Values')

# Save the histogram as a PNG image
output_image = 'hash_histogram.png'
plt.savefig(output_image, dpi=300, bbox_inches='tight')

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