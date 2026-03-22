$(document).ready(function () {
    // 使用事件委托处理删除按钮点击（性能优化：避免重复绑定）
    $('#fileList').on('click', '.delbtn', function () {
        var fileName = $(this).attr("fname");
        $.ajax({
            url: '/del?name=' + fileName,
            type: 'GET',
            processData: false,
            contentType: false,
            success: function (data) {
                refreshFileList();
            }
        });
    });

    // 刷新上传的文件列表
    function refreshFileList() {
        $.get('/files?limit=all', function (data) {
            $('#fileList').children().slice(1).remove();
            data.forEach(function (file) {
                $('#fileList').append(`
                <tr class="table-light">
                    <td class="myalign"><a href="/uploads/${file.name}" target="_blank">${file.name}</a></td>
                    <td class="myalign">${file.time}</td>
                    <td class="myalign">${file.size}</td>
                    <td class="myalign"><button fname="${file.name}" type="button" class="btn btn-outline-danger btn-sm delbtn">删除</button></td>
                </tr>
                `)
            });
        });
    }

    // 阻止浏览器默认打开文件的行为
    $(document).on('drop dragover', function (e) {
        e.preventDefault();
    });

    // 点击矩形区域触发文件选择
    $('#drop-area').on('click', function () {
        $('#fileInput').click();
    });

    // 当文件拖拽至矩形区域时
    $('#drop-area').on('drop', function (e) {
    	$("#progresscol").show();
    	e.preventDefault();
    	var files = e.originalEvent.dataTransfer.files; // 获取拖拽的文件列表
    	var chunkSize = 1024 * 1024; // 切片大小，这里以1MB为例
    	
    	// 创建上传队列
    	var uploadQueue = [];
    	for (var i = 0; i < files.length; i++) {
    		var file = files[i];
    		uploadQueue.push({
    			file: file,
    			chunks: Math.ceil(file.size / chunkSize),
    			currentChunk: 0,
    			progress: 0
    		});
    	}
    	
    	// 开始处理上传队列
    	processUploadQueue(uploadQueue, chunkSize);
    });

    // 监听文件选择事件
    $('#fileInput').on('change', function () {
        $('#fileInput').show();
    });

    // 点击上传按钮
    $('#uploadForm').submit(function (e) {
        $("#progresscol").show();
        e.preventDefault();
        var files = $('#fileInput')[0].files;
        var chunkSize = 1024 * 1024; // 切片大小，这里以1MB为例
        
        // 创建上传队列
        var uploadQueue = [];
        for (var i = 0; i < files.length; i++) {
            var file = files[i];
            uploadQueue.push({
                file: file,
                chunks: Math.ceil(file.size / chunkSize),
                currentChunk: 0,
                progress: 0
            });
        }
        
        // 开始处理上传队列
        processUploadQueue(uploadQueue, chunkSize);
    });

    // 处理上传队列
    function processUploadQueue(queue, chunkSize) {
    	if (queue.length === 0) {
    		setTimeout(refreshFileList, 2000);
    		setTimeout(() => {
    			$('#progresscol').hide();
    			$('#uploadProgress').css('width', '0%').text('0%');
    		}, 1000);
    		return;
    	}
    	
    	var item = queue[0];
    	
    	// 第一个文件上传前检查空间
    	if (item.currentChunk === 0) {
    		$.get('/check-space', { size: item.file.size, name: item.file.name }, function(res) {
    			if (res.ok) {
    				uploadChunk(item, queue, chunkSize);
    			} else {
    				// 空间不足，显示错误提示
    				alert('上传失败：' + res.message);
    				console.error('空间不足:', res.message);
    				// 跳过当前文件，继续处理队列
    				queue.shift();
    				processUploadQueue(queue, chunkSize);
    			}
    		}).fail(function() {
    			alert('无法检查存储空间，请稍后重试');
    			queue.shift();
    			processUploadQueue(queue, chunkSize);
    		});
    	} else {
    		uploadChunk(item, queue, chunkSize);
    	}
    }
   
    // 分块上传
    function uploadChunk(item, queue, chunkSize) {
    	var file = item.file;
    	var start = item.currentChunk * chunkSize;
    	var end = Math.min(file.size, start + chunkSize);
    	var chunk = file.slice(start, end);
   
    	var formData = new FormData();
    	formData.append('file', chunk);
    	formData.append('fileName', file.name);
    	formData.append('chunk', item.currentChunk);
    	formData.append('chunks', item.chunks);
   
    	$.ajax({
    		url: '/upload',
    		type: 'POST',
    		data: formData,
    		processData: false,
    		contentType: false,
    		xhr: function () {
    			var xhr = $.ajaxSettings.xhr();
    			if (xhr.upload) {
    				xhr.upload.addEventListener('progress', function (e) {
    					if (e.lengthComputable) {
    						var chunkProgress = (e.loaded / e.total);
    						var fileProgress = (item.currentChunk + chunkProgress) / item.chunks;
    						// 使用 slice(1) 排除当前正在上传的文件，避免进度重复计算
    						var completedProgress = queue.slice(1).reduce((sum, qItem) => sum + qItem.progress, 0);
    						var totalProgress = (completedProgress + fileProgress) / queue.length;
    						$('#uploadProgress').css('width', parseInt(totalProgress * 100) + '%').text(parseInt(totalProgress * 100) + '%');
    					}
    				}, false);
    			}
    			return xhr;
    		},
    		success: function (response) {
    			item.currentChunk++;
    			item.progress = item.currentChunk / item.chunks;
    			if (item.currentChunk < item.chunks) {
    				uploadChunk(item, queue, chunkSize);
    			} else {
    				queue.shift();
    				processUploadQueue(queue, chunkSize);
    			}
    		},
    		error: function () {
    			console.error('上传失败');
    			queue.shift();
    			processUploadQueue(queue, chunkSize);
    		}
    	});
    }

    refreshFileList();
}) 