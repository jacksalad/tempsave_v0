$(document).ready(function () {
    var currentDir = ''; // '' = root, relative path like 'a/b/c'

    // Restore directory from URL hash on load
    if (window.location.hash && window.location.hash.length > 1) {
        currentDir = decodeURIComponent(window.location.hash.substring(1));
    }

    // Listen for hash changes (browser back/forward)
    $(window).on('hashchange', function () {
        var newDir = window.location.hash.length > 1
            ? decodeURIComponent(window.location.hash.substring(1))
            : '';
        if (newDir !== currentDir) {
            currentDir = newDir;
            refreshFileList();
        }
    });

    // Helper: encode path segments while preserving '/'
    function encodePath(path) {
        if (!path) return '';
        return path.split('/').map(encodeURIComponent).join('/');
    }

    // Navigate to a directory
    function navigateToDir(dir) {
        currentDir = dir;
        window.location.hash = dir ? '#' + encodeURIComponent(dir) : '';
        refreshFileList();
    }

    // Render breadcrumb from currentDir
    function renderBreadcrumb() {
        var $bc = $('#breadcrumb').empty();
        // Root link
        $('<li>').addClass('breadcrumb-item').append(
            $('<a>').attr('href', '#').text('Root')
        ).appendTo($bc);
        if (currentDir) {
            var parts = currentDir.split('/');
            var accumulated = '';
            parts.forEach(function (part, index) {
                accumulated = accumulated ? accumulated + '/' + part : part;
                var $item = $('<li>').addClass('breadcrumb-item');
                if (index === parts.length - 1) {
                    $item.addClass('active').text(part);
                } else {
                    $('<a>').attr('href', '#' + encodeURIComponent(accumulated))
                        .text(part)
                        .appendTo($item);
                }
                $item.appendTo($bc);
            });
        }
    }

    // 使用事件委托处理删除按钮点击（性能优化：避免重复绑定）
    $('#fileList').on('click', '.delbtn', function (e) {
        e.stopPropagation(); // 防止触发文件夹导航
        var fileName = $(this).attr("fname");
        $.ajax({
            url: '/del',
            type: 'POST',
            data: { name: fileName, dir: currentDir },
            success: function (data) {
                refreshFileList();
            }
        });
    });

    // 刷新上传的文件列表
    function refreshFileList() {
        $.get('/files', { limit: 'all', dir: currentDir }, function (data) {
            var $list = $('#fileList');
            $list.children().slice(1).remove();
            data.forEach(function (file) {
                var $row = $('<tr>').addClass('table-light');
                if (file.type === 'dir') {
                    // 文件夹行：点击导航进入
                    $row.addClass('folder-row');
                    var $nameTd = $('<td>').addClass('myalign').text(file.name);
                    $row.on('click', function () {
                        var newDir = currentDir ? currentDir + '/' + file.name : file.name;
                        navigateToDir(newDir);
                    });
                    var $timeTd = $('<td>').addClass('myalign').text(file.time);
                    var $sizeTd = $('<td>').addClass('myalign').text('');
                    var $delTd = $('<td>').addClass('myalign');
                    // 文件夹删除按钮
                    var $delBtn = $('<button>').attr({
                        'fname': file.name,
                        'type': 'button'
                    }).addClass('btn btn-outline-danger btn-sm delfolderbtn').text('删除');
                    $delTd.append($delBtn);
                    $delBtn.on('click', function (e) {
                        e.stopPropagation();
                        var folderPath = currentDir ? currentDir + '/' + file.name : file.name;
                        if (confirm('确定删除文件夹 "' + folderPath + '" 吗？')) {
                            $.ajax({
                                url: '/rmdir',
                                type: 'POST',
                                data: { dir: folderPath },
                                success: function (res) {
                                    if (res.ok) {
                                        refreshFileList();
                                    } else {
                                        alert('删除失败: ' + res.message);
                                    }
                                },
                                error: function () {
                                    alert('删除文件夹失败');
                                }
                            });
                        }
                    });
                    $row.append($nameTd, $timeTd, $sizeTd, $delTd);
                } else {
                    // 文件行：已有的文件行逻辑
                    var fileUrl = currentDir
                        ? '/uploads/' + encodePath(currentDir) + '/' + encodeURIComponent(file.name)
                        : '/uploads/' + encodeURIComponent(file.name);
                    var $nameTd = $('<td>').addClass('myalign');
                    $('<a>').attr('href', fileUrl)
                        .attr('target', '_blank')
                        .text(file.name)
                        .appendTo($nameTd);
                    var $timeTd = $('<td>').addClass('myalign').text(file.time);
                    var $sizeTd = $('<td>').addClass('myalign').text(file.size);
                    var $delTd = $('<td>').addClass('myalign');
                    $('<button>').attr({
                        'fname': file.name,
                        'type': 'button'
                    }).addClass('btn btn-outline-danger btn-sm delbtn').text('删除').appendTo($delTd);

                    $row.append($nameTd, $timeTd, $sizeTd, $delTd);
                }
                $list.append($row);
            });
            renderBreadcrumb();
        });
    }

    // 新建文件夹按钮
    $('#newFolderBtn').on('click', function () {
        var folderName = prompt('请输入文件夹名称:');
        if (folderName && folderName.trim()) {
            var fullDir = currentDir ? currentDir + '/' + folderName.trim() : folderName.trim();
            $.ajax({
                url: '/mkdir',
                type: 'POST',
                data: { dir: fullDir },
                success: function (res) {
                    if (res.ok) {
                        refreshFileList();
                    } else {
                        alert('创建文件夹失败: ' + res.message);
                    }
                },
                error: function () {
                    alert('创建文件夹失败');
                }
            });
        }
    });

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
        var files = e.originalEvent.dataTransfer.files;
        var chunkSize = 1024 * 1024;

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
        var chunkSize = 1024 * 1024;

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

        processUploadQueue(uploadQueue, chunkSize);
    });

    // 处理上传队列
    function processUploadQueue(queue, chunkSize) {
        if (queue.length === 0) {
            setTimeout(refreshFileList, 2000);
            setTimeout(function () {
                $('#progresscol').hide();
                $('#uploadProgress').css('width', '0%').text('0%');
            }, 1000);
            return;
        }

        var item = queue[0];

        if (item.currentChunk === 0) {
            $.get('/check-space', { size: item.file.size, name: item.file.name, dir: currentDir }, function (res) {
                if (res.ok) {
                    uploadChunk(item, queue, chunkSize);
                } else {
                    alert('上传失败：' + res.message);
                    console.error('空间不足:', res.message);
                    queue.shift();
                    processUploadQueue(queue, chunkSize);
                }
            }).fail(function () {
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
        formData.append('fileSize', file.size);
        formData.append('dir', currentDir);

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
                            var completedProgress = queue.slice(1).reduce(function (sum, qItem) { return sum + qItem.progress; }, 0);
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
